// Copyright 2023 CUE Labs AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ociclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

func (c *client) Repositories(ctx context.Context) ociregistry.Iter[string] {
	// TODO paging
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:  ocirequest.ReqCatalogList,
		ListN: -1,
	})
	if err != nil {
		return ociregistry.ErrorIter[string](err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return ociregistry.ErrorIter[string](err)
	}
	var catalog struct {
		Repos []string `json:"repositories"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return ociregistry.ErrorIter[string](fmt.Errorf("cannot unmarshal catalog response: %v", err))
	}
	return ociregistry.SliceIter(catalog.Repos)
}

func (c *client) Tags(ctx context.Context, repoName string) ociregistry.Iter[string] {
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:  ocirequest.ReqTagsList,
		Repo:  repoName,
		ListN: 10000,
	})
	if err != nil {
		return ociregistry.ErrorIter[string](err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return ociregistry.ErrorIter[string](err)
	}
	var tagsResponse struct {
		Repo string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(data, &tagsResponse); err != nil {
		return ociregistry.ErrorIter[string](fmt.Errorf("cannot unmarshal tags list response: %v", err))
	}
	// TODO paging
	return ociregistry.SliceIter(tagsResponse.Tags)
}

func (c *client) Referrers(ctx context.Context, repoName string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	// TODO paging
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqReferrersList,
		Repo:   repoName,
		Digest: string(digest),
		ListN:  10000,
	})
	if err != nil {
		return ociregistry.ErrorIter[ociregistry.Descriptor](err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return ociregistry.ErrorIter[ociregistry.Descriptor](err)
	}
	var referrersResponse ocispec.Index
	if err := json.Unmarshal(data, &referrersResponse); err != nil {
		return ociregistry.ErrorIter[ociregistry.Descriptor](fmt.Errorf("cannot unmarshal referrers response: %v", err))
	}
	return ociregistry.SliceIter(referrersResponse.Manifests)
}
