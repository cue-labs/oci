// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ociserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/opencontainers/go-digest"
	ocispecroot "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rogpeppe/ociregistry"
)

var v2 = ocispecroot.Versioned{
	SchemaVersion: 2,
}

type catalog struct {
	Repos []string `json:"repositories"`
}

type listTags struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func (r *registry) handleManifestGet(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	var mr ociregistry.BlobReader
	var err error
	if rreq.tag != "" {
		mr, err = r.backend.GetTag(ctx, rreq.repo, rreq.tag)
	} else {
		mr, err = r.backend.GetManifest(ctx, rreq.repo, ociregistry.Digest(rreq.digest))
	}
	if err != nil {
		return err
	}
	desc := mr.Descriptor()
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	resp.Header().Set("Content-Type", desc.MediaType)
	resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, mr)
	return nil
}

func (r *registry) handleManifestHead(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	var desc ociregistry.Descriptor
	var err error
	if rreq.tag != "" {
		desc, err = r.backend.ResolveTag(ctx, rreq.repo, rreq.tag)
	} else {
		desc, err = r.backend.ResolveManifest(ctx, rreq.repo, ociregistry.Digest(rreq.digest))
	}
	if err != nil {
		return err
	}
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	resp.Header().Set("Content-Type", desc.MediaType)
	resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
	resp.WriteHeader(http.StatusOK)
	return nil
}

func (r *registry) handleManifestPut(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	mediaType := req.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	// TODO check that the media type is valid?
	// TODO size limit
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("cannot read content: %v", err)
	}
	dig := digest.FromBytes(data)
	var tag string
	if rreq.tag != "" {
		tag = rreq.tag
	} else {
		if ociregistry.Digest(rreq.digest) != dig {
			return ociregistry.ErrDigestInvalid
		}
	}
	desc, err := r.backend.PushManifest(ctx, rreq.repo, tag, data, mediaType)
	if err != nil {
		return err
	}
	// TODO OCI-Subject header?
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	resp.Header().Set("Location", "/v2/"+rreq.repo+"/manifests/"+string(desc.Digest))
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleManifestDelete(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	var err error
	if rreq.tag != "" {
		err = r.backend.DeleteTag(ctx, rreq.repo, rreq.tag)
	} else {
		err = r.backend.DeleteManifest(ctx, rreq.repo, ociregistry.Digest(rreq.digest))
	}
	if err != nil {
		return err
	}
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleTagsList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	// TODO we should be able to tell the backend to
	// start from a particular position to avoid fetching
	// all tags every time.
	tags, err := ociregistry.All(r.backend.Tags(ctx, rreq.repo))
	if err != nil {
		return err
	}
	sort.Strings(tags)

	// https://github.com/opencontainers/distribution-spec/blob/b505e9cc53ec499edbd9c1be32298388921bb705/detail.md#tags-paginated
	// Offset using last query parameter.
	if last := req.URL.Query().Get("last"); last != "" {
		for i, t := range tags {
			if t > last {
				tags = tags[i:]
				break
			}
		}
	}

	// Limit using n query parameter.
	if ns := req.URL.Query().Get("n"); ns != "" {
		if n, err := strconv.Atoi(ns); err != nil {
			return ociregistry.NewError("invalid value for query parameter n", ociregistry.ErrUnsupported.Code(), nil)
		} else if n < len(tags) {
			tags = tags[:n]
		}
	}

	tagsToList := listTags{
		Name: rreq.repo,
		Tags: tags,
	}

	msg, _ := json.Marshal(tagsToList)
	resp.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, bytes.NewReader([]byte(msg)))
	return nil
}
func (r *registry) handleCatalogList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	n := 10000
	if rreq.listN >= 0 {
		n = rreq.listN
	}
	repos, err := ociregistry.All(r.backend.Repositories(ctx))
	if err != nil {
		return err
	}
	// TODO: implement pagination
	if len(repos) > n {
		repos = repos[:n]
	}
	msg, err := json.Marshal(catalog{
		Repos: repos,
	})
	if err != nil {
		return err
	}
	resp.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, bytes.NewReader([]byte(msg)))
	return nil
}

// TODO: implement handling of artifactType querystring
func (r *registry) handleReferrersList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	im := &ocispec.Index{
		Versioned: v2,
		MediaType: mediaTypeOCIImageIndex,
	}

	// TODO support artifactType filtering
	it := r.backend.Referrers(ctx, rreq.repo, ociregistry.Digest(rreq.digest), "")
	for {
		desc, ok := it.Next()
		if !ok {
			break
		}
		im.Manifests = append(im.Manifests, desc)
	}
	if err := it.Error(); err != nil {
		return err
	}
	msg, err := json.Marshal(im)
	if err != nil {
		return err
	}
	resp.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	resp.WriteHeader(http.StatusOK)
	resp.Write(msg)
	return nil
}
