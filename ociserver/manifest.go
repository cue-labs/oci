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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

type manifests struct {
	backend ociregistry.Interface
}

// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pulling-an-image-manifest
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pushing-an-image
func (m *manifests) handle(resp http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	target := elem[len(elem)-1]
	repo := strings.Join(elem[1:len(elem)-2], "/")

	switch req.Method {
	case http.MethodGet:
		var r ociregistry.BlobReader
		var err error
		switch {
		case isValidDigest(target):
			r, err = m.backend.GetManifest(ctx, repo, ociregistry.Digest(target))
		case isValidTag(target):
			r, err = m.backend.GetTag(ctx, repo, target)
		default:
			return ociregistry.ErrDigestInvalid
		}
		if err != nil {
			return err
		}
		desc := r.Descriptor()
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.WriteHeader(http.StatusOK)
		io.Copy(resp, r)
		return nil

	case http.MethodHead:
		var desc ociregistry.Descriptor
		var err error
		switch {
		case isValidDigest(target):
			desc, err = m.backend.ResolveManifest(ctx, repo, ociregistry.Digest(target))
		case isValidTag(target):
			desc, err = m.backend.ResolveTag(ctx, repo, target)
		default:
			return ociregistry.ErrDigestInvalid
		}
		if err != nil {
			return err
		}
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.WriteHeader(http.StatusOK)
		return nil

	case http.MethodPut:
		mediaType := req.Header.Get("Content-Type")
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		// TODO check that the media type is valid?
		// TODO size limit
		data, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("cannot read content: %v", err)
		}
		dig := digest.FromBytes(data)
		var tag string
		switch {
		case isValidDigest(target):
			if ociregistry.Digest(target) != dig {
				return ociregistry.ErrDigestInvalid
			}
		case isValidTag(target):
			tag = target
		default:
			return ociregistry.ErrDigestInvalid
		}
		desc, err := m.backend.PushManifest(ctx, repo, tag, data, mediaType)
		if err != nil {
			return err
		}
		// TODO OCI-Subject header?
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.WriteHeader(http.StatusCreated)
		return nil

	case http.MethodDelete:
		var err error
		switch {
		case isValidDigest(target):
			err = m.backend.DeleteManifest(ctx, repo, ociregistry.Digest(target))
		case isValidTag(target):
			err = m.backend.DeleteTag(ctx, repo, target)
		default:
			return ociregistry.ErrDigestInvalid
		}
		if err != nil {
			return err
		}
		resp.WriteHeader(http.StatusAccepted)
		return nil

	default:
		return errMethodNotAllowed
	}
}

func (m *manifests) handleTags(resp http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	repo := strings.Join(elem[1:len(elem)-2], "/")

	if req.Method == "GET" {
		// TODO we should be able to tell the backend to
		// start from a particular position to avoid fetching
		// all tags every time.
		tags, err := ociregistry.All(m.backend.Tags(ctx, repo))
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
			Name: repo,
			Tags: tags,
		}

		msg, _ := json.Marshal(tagsToList)
		resp.Header().Set("Content-Length", fmt.Sprint(len(msg)))
		resp.WriteHeader(http.StatusOK)
		io.Copy(resp, bytes.NewReader([]byte(msg)))
		return nil
	}

	return errMethodNotAllowed
}

func (m *manifests) handleCatalog(resp http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	query := req.URL.Query()
	nStr := query.Get("n")
	n := 10000
	if nStr != "" {
		n, _ = strconv.Atoi(nStr)
	}

	if req.Method == "GET" {
		repos, err := ociregistry.All(m.backend.Repositories(ctx))
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

	return errMethodNotAllowed
}

// TODO: implement handling of artifactType querystring
func (m *manifests) handleReferrers(resp http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	// Ensure this is a GET request
	if req.Method != "GET" {
		return errMethodNotAllowed
	}

	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	target := elem[len(elem)-1]
	repo := strings.Join(elem[1:len(elem)-2], "/")

	// Validate that incoming target is a valid digest
	if !isValidDigest(target) {
		return ociregistry.ErrDigestInvalid
	}
	im := &ocispec.Index{
		Versioned: v2,
		MediaType: mediaTypeOCIImageIndex,
	}

	// TODO support artifactType filtering
	it := m.backend.Referrers(ctx, repo, ociregistry.Digest(target), "")
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
