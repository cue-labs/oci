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
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/rogpeppe/ociregistry"
)

// blobs
type blobs struct {
	backend ociregistry.Interface

	lock sync.Mutex
	log  *log.Logger
}

func (b *blobs) handle(resp http.ResponseWriter, req *http.Request, rreq *registryRequest) error {
	ctx := req.Context()

	switch rreq.kind {
	case reqBlobHead:
		desc, err := b.backend.ResolveBlob(ctx, rreq.repo, ociregistry.Digest(rreq.digest))
		if err != nil {
			return err
		}
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.WriteHeader(http.StatusOK)
		return nil

	case reqBlobGet:
		blob, err := b.backend.GetBlob(ctx, rreq.repo, ociregistry.Digest(rreq.digest))
		if err != nil {
			return err
		}
		defer blob.Close()
		desc := blob.Descriptor()
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.Header().Set("Docker-Content-Digest", rreq.digest)
		resp.WriteHeader(http.StatusOK)

		io.Copy(resp, blob)
		return nil
	case reqBlobUploadBlob:
		// TODO check that Content-Type is application/octet-stream?
		mediaType := "application/octet-stream"

		desc, err := b.backend.PushBlob(req.Context(), rreq.repo, ociregistry.Descriptor{
			MediaType: mediaType,
			Size:      req.ContentLength,
			Digest:    ociregistry.Digest(rreq.digest),
		}, req.Body)
		if err != nil {
			return err
		}
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.WriteHeader(http.StatusCreated)
		return nil

	case reqBlobStartUpload:
		w, err := b.backend.PushBlobChunked(ctx, rreq.repo, "")
		if err != nil {
			return err
		}
		defer w.Close()
		log.Printf("started initial PushBlobChunked (id %q)", w.ID())
		// TODO how can we make it so that the backend can return a location that isn't
		// in the registry?
		resp.Header().Set("Location", "/v2/"+rreq.repo+"/blobs/uploads/"+w.ID())
		resp.Header().Set("Range", "0-0")
		resp.WriteHeader(http.StatusAccepted)
		return nil

	case reqBlobUploadChunk:
		// TODO technically it seems like there should always be
		// a content range for a PATCH request but the existing tests
		// seem to be lax about it, and we can just assume for the
		// first patch that the range is 0-(contentLength-1)
		start := int64(0)
		contentRange := req.Header.Get("Content-Range")
		if contentRange != "" {
			var end int64
			if n, err := fmt.Sscanf(contentRange, "%d-%d", &start, &end); err != nil || n != 2 {
				return badAPIUseError("We don't understand your Content-Range")
			}
		}
		w, err := b.backend.PushBlobChunked(ctx, rreq.repo, rreq.uploadID)
		if err != nil {
			return err
		}
		defer w.Close()
		// TODO this is potentially racy if multiple clients are doing this concurrently.
		// Perhaps the PushBlobChunked call should take a "startAt" parameter?
		if start != w.Size() {
			return fmt.Errorf("write at invalid starting point %d; actual start %d: %w", start, w.Size(), withHTTPCode(http.StatusRequestedRangeNotSatisfiable, ociregistry.ErrBlobUploadInvalid))
		}
		if n, err := io.Copy(w, req.Body); err != nil {
			return fmt.Errorf("cannot copy blob data: %v", err)
		} else {
			log.Printf("copied %d bytes to blob", n)
		}

		resp.Header().Set("Location", "/v2/"+rreq.repo+"/blobs/uploads/"+rreq.uploadID)
		resp.Header().Set("Range", fmt.Sprintf("0-%d", w.Size()-1))
		resp.WriteHeader(http.StatusNoContent)
		return nil

	case reqBlobCompleteUpload:
		w, err := b.backend.PushBlobChunked(ctx, rreq.repo, rreq.uploadID)
		if err != nil {
			return err
		}
		defer w.Close()

		if _, err := io.Copy(w, req.Body); err != nil {
			return fmt.Errorf("failed to copy data: %v", err)
		}
		digest, err := w.Commit(ctx, ociregistry.Digest(rreq.digest))
		if err != nil {
			return err
		}
		resp.Header().Set("Docker-Content-Digest", string(digest))
		resp.WriteHeader(http.StatusCreated)
		return nil

	case reqBlobDelete:
		if err := b.backend.DeleteBlob(ctx, rreq.repo, ociregistry.Digest(rreq.digest)); err != nil {
			return err
		}
		resp.WriteHeader(http.StatusAccepted)
		return nil

	default:
		return errMethodNotAllowed
	}
}
