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

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
)

func (r *registry) handleBlobHead(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	desc, err := r.backend.ResolveBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
	if err != nil {
		return err
	}
	resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	resp.WriteHeader(http.StatusOK)
	return nil
}

func (r *registry) handleBlobGet(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
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
}

func (r *registry) handleBlobUploadBlob(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// TODO check that Content-Type is application/octet-stream?
	mediaType := "application/octet-stream"

	desc, err := r.backend.PushBlob(req.Context(), rreq.Repo, ociregistry.Descriptor{
		MediaType: mediaType,
		Size:      req.ContentLength,
		Digest:    ociregistry.Digest(rreq.Digest),
	}, req.Body)
	if err != nil {
		return err
	}
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	resp.Header().Set("Location", "/v2/"+rreq.Repo+"/blobs/"+string(desc.Digest))
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleBlobStartUpload(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	w, err := r.backend.PushBlobChunked(ctx, rreq.Repo, "", 0)
	if err != nil {
		return err
	}
	defer w.Close()

	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", "0-0")
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleBlobUploadInfo(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	w, err := r.backend.PushBlobChunked(ctx, rreq.Repo, rreq.UploadID, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", rangeString(0, w.Size()))
	resp.WriteHeader(http.StatusNoContent)
	return nil
}

func (r *registry) handleBlobUploadChunk(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
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
	w, err := r.backend.PushBlobChunked(ctx, rreq.Repo, rreq.UploadID, 0)
	if err != nil {
		return err
	}
	// TODO this is potentially racy if multiple clients are doing this concurrently.
	// Perhaps the PushBlobChunked call should take a "startAt" parameter?
	if start != w.Size() {
		return fmt.Errorf("write at invalid starting point %d; actual start %d: %w", start, w.Size(), withHTTPCode(http.StatusRequestedRangeNotSatisfiable, ociregistry.ErrBlobUploadInvalid))
	}
	if _, err := io.Copy(w, req.Body); err != nil {
		w.Close()
		return fmt.Errorf("cannot copy blob data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("cannot close BlobWriter: %w", err)
	}
	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", rangeString(0, w.Size()))
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleBlobCompleteUpload(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	w, err := r.backend.PushBlobChunked(ctx, rreq.Repo, rreq.UploadID, 0)
	if err != nil {
		return err
	}
	defer w.Close()

	if _, err := io.Copy(w, req.Body); err != nil {
		return fmt.Errorf("failed to copy data to %T: %v", w, err)
	}
	digest, err := w.Commit(ociregistry.Digest(rreq.Digest))
	if err != nil {
		return err
	}
	resp.Header().Set("Docker-Content-Digest", string(digest))
	resp.Header().Set("Location", "/v2/"+rreq.Repo+"/blobs/"+string(digest))
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleBlobMount(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if err := r.backend.MountBlob(ctx, rreq.FromRepo, rreq.Repo, ociregistry.Digest(rreq.Digest)); err != nil {
		return err
	}
	resp.Header().Set("Location", "/v2/"+rreq.Repo+"/blobs/"+rreq.Digest)
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleBlobDelete(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if err := r.backend.DeleteBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest)); err != nil {
		return err
	}
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) locationForUploadID(repo string, uploadID string) string {
	_, loc := (&ocirequest.Request{
		Kind:     ocirequest.ReqBlobUploadInfo,
		Repo:     repo,
		UploadID: uploadID,
	}).Construct()
	return loc
}

func rangeString(x0, x1 int64) string {
	x1--
	if x1 < 0 {
		x1 = 0
	}
	return fmt.Sprintf("%d-%d", x0, x1)
}
