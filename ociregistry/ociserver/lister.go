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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

type catalog struct {
	Repos []string `json:"repositories"`
}

type listTags struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func (r *registry) handleTagsList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// TODO we should be able to tell the backend to
	// start from a particular position to avoid fetching
	// all tags every time.
	tags, err := ociregistry.All(r.backend.Tags(ctx, rreq.Repo))
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
		Name: rreq.Repo,
		Tags: tags,
	}

	msg, _ := json.Marshal(tagsToList)
	resp.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, bytes.NewReader([]byte(msg)))
	return nil
}

func (r *registry) handleCatalogList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	n := 10000
	if rreq.ListN >= 0 {
		n = rreq.ListN
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
func (r *registry) handleReferrersList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if r.opts.DisableReferrersAPI {
		return withHTTPCode(http.StatusNotFound, fmt.Errorf("referrers API has been disabled"))
	}

	im := &ocispec.Index{
		Versioned: v2,
		MediaType: mediaTypeOCIImageIndex,
	}

	// TODO support artifactType filtering
	it := r.backend.Referrers(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest), "")
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
	resp.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	resp.WriteHeader(http.StatusOK)
	resp.Write(msg)
	return nil
}
