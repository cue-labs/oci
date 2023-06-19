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
	"context"
	"fmt"
	"io"
	"net/http"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

func (r *registry) handleBlobHead(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	desc, err := r.backend.ResolveBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
	if err != nil {
		return err
	}
	resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	// TODO this is true in theory, but what if the backend doesn't support GetBlobRange ?
	resp.Header().Set("Accept-Ranges", "bytes")
	resp.WriteHeader(http.StatusOK)
	return nil
}

func (r *registry) handleBlobGet(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if r.opts.LocationsForDescriptor != nil {
		// We need to find information on the blob before we can determine
		// what to pass back, so resolve the blob first so we don't
		// stimulate the backend to start sending the whole stream
		// only to abandon it.
		desc, err := r.backend.ResolveBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
		if err != nil {
			// TODO this might not be the best response because ResolveBlob is
			// often implemented with a HEAD request that can't return an error
			// body. So it might be better to fall through to the usual GetBlob request,
			// although that would mean that every error makes two calls :(
			return err
		}
		locs, err := r.opts.LocationsForDescriptor(false, desc)
		if err != nil {
			return err
		}
		if len(locs) > 0 {
			// TODO choose randomly from the set of locations?
			// TODO make it possible to turn off this behaviour?
			http.Redirect(resp, req, locs[0], http.StatusTemporaryRedirect)
			return nil
		}
	}
	ranges, err := parseRange(req.Header.Get("Range"))
	if err != nil {
		return withHTTPCode(http.StatusRequestedRangeNotSatisfiable, err)
	}
	switch len(ranges) {
	case 0:
		blob, err := r.backend.GetBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
		if err != nil {
			return err
		}
		defer blob.Close()
		desc := blob.Descriptor()
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.Header().Set("Docker-Content-Digest", rreq.Digest)
		resp.WriteHeader(http.StatusOK)

		io.Copy(resp, blob)
		return nil
	case 1:
		rng := ranges[0]
		blob, err := r.backend.GetBlobRange(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest), rng.start, rng.end)
		if err != nil {
			// TODO fall back to using GetBlob if err is ErrUnsupported?
			return err
		}
		defer blob.Close()
		desc := blob.Descriptor()
		if rng.end == -1 || rng.end > desc.Size {
			rng.end = desc.Size
		}
		if rng.start > desc.Size {
			return withHTTPCode(http.StatusRequestedRangeNotSatisfiable, fmt.Errorf("range starts after end of blob"))
		}
		if rng.end < rng.start {
			return withHTTPCode(http.StatusRequestedRangeNotSatisfiable, fmt.Errorf("range end is before start"))
		}
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(rng.end-rng.start))
		resp.Header().Set("Docker-Content-Digest", rreq.Digest)
		resp.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rng.start, rng.end-1, desc.Size))
		resp.WriteHeader(http.StatusPartialContent)

		io.Copy(resp, blob)
		return nil

	default:
		return withHTTPCode(http.StatusRequestedRangeNotSatisfiable, fmt.Errorf("only a single range is supported"))
	}
}

func (r *registry) handleManifestGet(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// TODO we could do a redirect here too if we thought it was worthwhile.
	var mr ociregistry.BlobReader
	var err error
	if rreq.Tag != "" {
		mr, err = r.backend.GetTag(ctx, rreq.Repo, rreq.Tag)
	} else {
		mr, err = r.backend.GetManifest(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
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

func (r *registry) handleManifestHead(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	var desc ociregistry.Descriptor
	var err error
	if rreq.Tag != "" {
		desc, err = r.backend.ResolveTag(ctx, rreq.Repo, rreq.Tag)
	} else {
		desc, err = r.backend.ResolveManifest(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
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
