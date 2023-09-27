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
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

func (r *registry) handleBlobUploadBlob(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if r.opts.DisableSinglePostUpload {
		return r.handleBlobStartUpload(ctx, resp, req, rreq)
	}
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
	if err := r.setLocationHeader(resp, false, desc, "/v2/"+rreq.Repo+"/blobs/"+string(desc.Digest)); err != nil {
		return err
	}
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
	desc, err := w.Commit(ociregistry.Digest(rreq.Digest))
	if err != nil {
		return err
	}
	if err := r.setLocationHeader(resp, false, desc, "/v2/"+rreq.Repo+"/blobs/"+string(desc.Digest)); err != nil {
		return err
	}
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleBlobMount(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	desc, err := r.backend.MountBlob(ctx, rreq.FromRepo, rreq.Repo, ociregistry.Digest(rreq.Digest))
	if err != nil {
		return err
	}
	if err := r.setLocationHeader(resp, true, desc, "/v2/"+rreq.Repo+"/blobs/"+rreq.Digest); err != nil {
		return err
	}
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func (r *registry) handleManifestPut(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
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
	if rreq.Tag != "" {
		tag = rreq.Tag
	} else {
		if ociregistry.Digest(rreq.Digest) != dig {
			return ociregistry.ErrDigestInvalid
		}
	}
	subjectDesc, err := subjectFromManifest(req.Header.Get("Content-Type"), data)
	if err != nil {
		return fmt.Errorf("invalid manifest JSON: %v", err)
	}
	desc, err := r.backend.PushManifest(ctx, rreq.Repo, tag, data, mediaType)
	if err != nil {
		return err
	}
	if err := r.setLocationHeader(resp, false, desc, "/v2/"+rreq.Repo+"/manifests/"+string(desc.Digest)); err != nil {
		return err
	}
	if subjectDesc != nil {
		resp.Header().Set("OCI-Subject", string(subjectDesc.Digest))
	}
	// TODO OCI-Subject header?
	resp.WriteHeader(http.StatusCreated)
	return nil
}

func subjectFromManifest(contentType string, data []byte) (*ociregistry.Descriptor, error) {
	switch contentType {
	case ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex:
		break
		// TODO other manifest media types.
	default:
		return nil, nil
	}
	var m struct {
		Subject *ociregistry.Descriptor `json:"subject"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m.Subject, nil
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
