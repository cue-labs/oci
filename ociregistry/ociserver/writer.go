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
	"strconv"

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
	mediaType := mediaTypeOctetStream

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
	// Start a chunked upload. When r.backend is ociclient, this should
	// just result in a single POST request that starts the upload.
	w, err := r.backend.PushBlobChunked(ctx, rreq.Repo, 0)
	if err != nil {
		return err
	}
	defer w.Close()

	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", "0-0")
	// TODO: reject chunks which don't follow this minimum length.
	// If any reasonable clients are broken by this, we can always reconsider,
	// perhaps by making the strictness on chunk sizes opt-in.
	resp.Header().Set("OCI-Chunk-Min-Length", strconv.Itoa(w.ChunkSize()))
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleBlobUploadInfo(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// Resume the upload without actually writing to it, passing -1 for the offset
	// to cause the backend to retrieve the associated upload information.
	// When r.backend is ociclient, this should result in a single GET request
	// to retrieve upload info.
	w, err := r.backend.PushBlobChunkedResume(ctx, rreq.Repo, rreq.UploadID, -1, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", ocirequest.RangeString(0, w.Size()))
	resp.WriteHeader(http.StatusNoContent)
	return nil
}

func (r *registry) handleBlobUploadChunk(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// Note that the spec requires chunked upload PATCH requests to include Content-Range,
	// but the conformance tests do not actually follow that as of the time of writing.
	// Allow the missing header to result in start=0, meaning we assume it's the first chunk.
	start, end, err := chunkRange(req)
	if err != nil {
		return err
	}

	w, err := r.backend.PushBlobChunkedResume(ctx, rreq.Repo, rreq.UploadID, start, int(end-start))
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, req.Body); err != nil {
		w.Close()
		return fmt.Errorf("cannot copy blob data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("cannot close BlobWriter: %w", err)
	}
	resp.Header().Set("Location", r.locationForUploadID(rreq.Repo, w.ID()))
	resp.Header().Set("Range", ocirequest.RangeString(0, w.Size()))
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleBlobCompleteUpload(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	// We are handling a PUT as part of one of:
	//
	// 1) An entire blob via POST-then-PUT.
	// 2) The last chunk of a chunked upload as part of the closing PUT, with a valid Content-Range.
	// 3) Closing a finished chunked upload with an empty-bodied PUT.
	//
	// We can't actually tell these apart upfront;
	// for example, 3 can have an octet-stream content type even though it has no body,
	// meaning that it looks exactly like 1, as seen in the conformance tests.
	// For that reason, we simply forward the range start as the offset in case 2,
	// while using an offset of 0 in cases 1 and 3 without a range, to avoid a GET in ociclient.
	//
	// Note that we don't check "ok" here, letting "start" default to 0 due to the above.
	start, end, err := chunkRange(req)
	if err != nil {
		return err
	}

	w, err := r.backend.PushBlobChunkedResume(ctx, rreq.Repo, rreq.UploadID, start, int(end-start))
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
		mediaType = mediaTypeOctetStream
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
	}).MustConstruct()
	return loc
}

func chunkRange(req *http.Request) (start, end int64, _ error) {
	var rangeOK bool
	if s := req.Header.Get("Content-Range"); s != "" {
		start, end, rangeOK = ocirequest.ParseRange(s)
		if !rangeOK {
			return 0, 0, badAPIUseError("we don't understand your Content-Range")
		}
	}

	if rangeOK && req.ContentLength >= 0 {
		rangeLength := end - start
		if rangeLength != req.ContentLength {
			return 0, 0, badAPIUseError("Content-Range implies a length of %d but Content-Length is %d", rangeLength, req.ContentLength)
		}
	}

	// The registry here is stateless, so it doesn't remember what minimum chunk size
	// the backend registry suggested that we should use.
	// We rely on the HTTP client to remember that minimum and use it,
	// which would mean that each PATCH chunk before the last should be at least as large.
	// Extract that size from either Content-Range or Content-Length;
	// if neither is set, we fall back to 0, letting the backend assume a default.
	if !rangeOK && req.ContentLength >= 0 {
		end = req.ContentLength
	}
	return start, end, nil
}
