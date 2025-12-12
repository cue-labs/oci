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
	"iter"
	"net/http"
	"net/url"
	"strconv"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

const maxPageSize = 10000

type catalog struct {
	Repos []string `json:"repositories"`
}

type listTags struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func (r *registry) handleTagsList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	tags, link, err := r.nextListResults(req, rreq, r.backend.Tags(ctx, rreq.Repo, rreq.ListLast))
	if err != nil {
		return err
	}
	msg, _ := json.Marshal(listTags{
		Name: rreq.Repo,
		Tags: tags,
	})
	if link != "" {
		resp.Header().Set("Link", link)
	}
	resp.Header().Set("Content-Length", strconv.Itoa(len(msg)))
	resp.WriteHeader(http.StatusOK)
	resp.Write(msg)
	return nil
}

func (r *registry) handleCatalogList(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) (_err error) {
	repos, link, err := r.nextListResults(req, rreq, r.backend.Repositories(ctx, rreq.ListLast))
	if err != nil {
		return err
	}
	msg, err := json.Marshal(catalog{
		Repos: repos,
	})
	if err != nil {
		return err
	}
	if link != "" {
		resp.Header().Set("Link", link)
	}
	resp.Header().Set("Content-Length", strconv.Itoa(len(msg)))
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, bytes.NewReader([]byte(msg)))
	return nil
}

func (r *registry) handleReferrersList(ctx context.Context, resp http.ResponseWriter, _ *http.Request, rreq *ocirequest.Request) error {
	if r.opts.DisableReferrersAPI {
		return withHTTPCode(http.StatusNotFound, fmt.Errorf("referrers API has been disabled"))
	}
	artifactType := rreq.ArtifactType
	if r.opts.DisableReferrersFiltering {
		artifactType = ""
	}

	im := &ocispec.Index{
		Versioned: v2,
		MediaType: mediaTypeOCIImageIndex,
	}

	// TODO this could potentially end up with a very large response which we might
	// want to limit. The spec does provide with a means to let a server respond with a partial
	// request, linked to the next one with a Link header. However, arranging that is non-trivial
	// because we'd need a way to return a link value to the client that enables a fresh
	// call to Referrers to start where the old one left off. For now, we'll punt.
	for desc, err := range r.backend.Referrers(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest), artifactType) {
		if err != nil {
			return err
		}
		im.Manifests = append(im.Manifests, desc)
	}
	msg, err := json.Marshal(im)
	if err != nil {
		return err
	}
	resp.Header().Set("Content-Length", strconv.Itoa(len(msg)))
	resp.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	if artifactType != "" {
		resp.Header().Set("OCI-Filters-Applied", "artifactType")
	}
	resp.WriteHeader(http.StatusOK)
	resp.Write(msg)
	return nil
}

func (r *registry) nextListResults(req *http.Request, rreq *ocirequest.Request, itemsIter iter.Seq2[string, error]) (items []string, link string, _ error) {
	if r.opts.MaxListPageSize > 0 && rreq.ListN > r.opts.MaxListPageSize {
		return nil, "", ociregistry.NewError(fmt.Sprintf("query parameter n is too large (n=%d, max=%d)", rreq.ListN, r.opts.MaxListPageSize), ociregistry.ErrUnsupported.Code(), nil)
	}
	n := rreq.ListN
	if n <= 0 {
		n = maxPageSize
	}
	truncated := false
	for item, err := range itemsIter {
		if err != nil {
			return nil, "", err
		}
		if rreq.ListN > 0 && len(items) >= rreq.ListN {
			truncated = true
			break
		}
		// TODO we might want some way to limit on the total number
		// of items returned in the absence of a ListN limit.
		items = append(items, item)
		// TODO sanity check that the items are in lexical order?
	}
	if truncated && !r.opts.OmitLinkHeaderFromResponses {
		link = r.makeNextLink(req, items[len(items)-1])
	}
	return items, link, nil
}

// makeNextLink returns an RFC 5988 Link value suitable for
// providing the next URL in a chain of list page results,
// starting after the given "startAfter" item.
// TODO this assumes that req.URL.Path is the actual
// path that the client used to access the server. This might
// not necessarily be true, so maybe it would be better to
// use a path-relative URL instead, although that's trickier
// to arrange.
func (r *registry) makeNextLink(req *http.Request, startAfter string) string {
	// Use the "next" relation type:
	// See https://html.spec.whatwg.org/multipage/links.html#link-type-next
	query := req.URL.Query()
	query.Set("last", startAfter)
	u := &url.URL{
		Path:     req.URL.Path,
		RawQuery: query.Encode(),
	}
	return fmt.Sprintf(`<%v>;rel="next"`, u)
}
