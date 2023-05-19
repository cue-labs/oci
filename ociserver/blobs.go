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
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/hasher"
)

// Returns whether this url should be handled by the blob handler
// This is complicated because blob is indicated by the trailing path, not the leading path.
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pulling-a-layer
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pushing-a-layer
func isBlob(req *http.Request) bool {
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	if elem[len(elem)-1] == "" {
		elem = elem[:len(elem)-1]
	}
	if len(elem) < 3 {
		return false
	}
	return elem[len(elem)-2] == "blobs" || (elem[len(elem)-3] == "blobs" &&
		elem[len(elem)-2] == "uploads")
}

// redirectError represents a signal that the blob handler doesn't have the blob
// contents, but that those contents are at another location which registry
// clients should redirect to.
type redirectError struct {
	// Location is the location to find the contents.
	Location string

	// Code is the HTTP redirect status code to return to clients.
	Code int
}

func (e redirectError) Error() string { return fmt.Sprintf("redirecting (%d): %s", e.Code, e.Location) }

// errNotFound represents an error locating the blob.
var errNotFound = errors.New("not found")

// blobs
type blobs struct {
	backend ociregistry.Interface

	lock sync.Mutex
	log  *log.Logger
}

func (b *blobs) handle(resp http.ResponseWriter, req *http.Request) *regError {
	ctx := req.Context()
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	if elem[len(elem)-1] == "" {
		elem = elem[:len(elem)-1]
	}
	// Must have a path of form /v2/{name}/blobs/{upload,sha256:}
	if len(elem) < 4 {
		return &regError{
			Status:  http.StatusBadRequest,
			Code:    "NAME_INVALID",
			Message: "blobs must be attached to a repo",
		}
	}
	target := elem[len(elem)-1]
	service := elem[len(elem)-2]
	digest := ociregistry.Digest(req.URL.Query().Get("digest"))
	contentRange := req.Header.Get("Content-Range")

	repo := req.URL.Host + path.Join(elem[1:len(elem)-2]...)

	switch req.Method {
	case http.MethodHead:
		_, err := hasher.NewHash(target)
		if err != nil {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "NAME_INVALID",
				Message: "invalid digest",
			}
		}
		desc, err := b.backend.ResolveBlob(ctx, repo, ociregistry.Digest(target))
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "TODO",
				Message: "cannot resolve digest",
			}
		}
		// TODO
		//		if errors.Is(err, errNotFound) {
		//			return regErrBlobUnknown
		//		} else if err != nil {
		//			var rerr redirectError
		//			if errors.As(err, &rerr) {
		//				http.Redirect(resp, req, rerr.Location, rerr.Code)
		//				return nil
		//			}
		//			return regErrInternal(err)
		//		}

		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
		resp.WriteHeader(http.StatusOK)
		return nil

	case http.MethodGet:
		h, err := hasher.NewHash(target)
		if err != nil {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "NAME_INVALID",
				Message: "invalid digest",
			}
		}

		blob, err := b.backend.GetBlob(ctx, repo, ociregistry.Digest(target))
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "TODO",
				Message: "cannot get blob",
			}
		}
		defer blob.Close()
		desc := blob.Descriptor()
		resp.Header().Set("Content-Type", desc.MediaType)
		resp.Header().Set("Content-Length", fmt.Sprint(desc.Size))
		resp.Header().Set("Docker-Content-Digest", h.String())
		resp.WriteHeader(http.StatusOK)

		// TODO
		//			if errors.Is(err, errNotFound) {
		//				return regErrBlobUnknown
		//			} else if err != nil {
		//				var rerr redirectError
		//				if errors.As(err, &rerr) {
		//					http.Redirect(resp, req, rerr.Location, rerr.Code)
		//					return nil
		//				}
		//
		//				return regErrInternal(err)
		//			}

		io.Copy(resp, blob)
		return nil

	case http.MethodPost:

		// It is weird that this is "target" instead of "service", but
		// that's how the index math works out above.
		if target != "uploads" {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "METHOD_UNKNOWN",
				Message: fmt.Sprintf("POST to /blobs must be followed by /uploads, got %s", target),
			}
		}

		if digest != "" {
			h, err := hasher.NewHash(string(digest))
			if err != nil {
				return regErrDigestInvalid
			}

			vrc, err := hasher.ReadCloser(req.Body, req.ContentLength, h)
			if err != nil {
				return regErrInternal(err)
			}
			defer vrc.Close()

			desc, err := b.backend.PushBlob(req.Context(), repo, ociregistry.Descriptor{
				MediaType: req.Header.Get("Content-Type"),
				Size:      req.ContentLength,
				Digest:    digest,
			}, vrc)
			if err != nil {
				if errors.As(err, &hasher.Error{}) {
					return regErrDigestMismatch
				}
				return regErrInternal(err)
			}
			resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
			resp.WriteHeader(http.StatusCreated)
			return nil
		}
		w, err := b.backend.PushBlobChunked(ctx, repo, "")
		if err != nil {
			return errTODO()
		}

		// TODO how can we make it so that the backend can return a location that isn't
		// in the registry?
		resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-2]...), "blobs/uploads", w.ID()))
		resp.Header().Set("Range", "0-0")
		resp.WriteHeader(http.StatusAccepted)
		w.Close()
		return nil

	case http.MethodPatch:
		if service != "uploads" {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "METHOD_UNKNOWN",
				Message: fmt.Sprintf("PATCH to /blobs must be followed by /uploads, got %s", service),
			}
		}

		if contentRange != "" {
			return errTODO()
			//			start, end := 0, 0
			//			if _, err := fmt.Sscanf(contentRange, "%d-%d", &start, &end); err != nil {
			//				return &regError{
			//					Status:  http.StatusRequestedRangeNotSatisfiable,
			//					Code:    "BLOB_UPLOAD_UNKNOWN",
			//					Message: "We don't understand your Content-Range",
			//				}
			//			}
			//			b.lock.Lock()
			//			defer b.lock.Unlock()
			//			if start != len(b.uploads[target]) {
			//				return &regError{
			//					Status:  http.StatusRequestedRangeNotSatisfiable,
			//					Code:    "BLOB_UPLOAD_UNKNOWN",
			//					Message: "Your content range doesn't match what we have",
			//				}
			//			}
			//			l := bytes.NewBuffer(b.uploads[target])
			//			io.Copy(l, req.Body)
			//			b.uploads[target] = l.Bytes()
			//			resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-3]...), "blobs/uploads", target))
			//			resp.Header().Set("Range", fmt.Sprintf("0-%d", len(l.Bytes())-1))
			//			resp.WriteHeader(http.StatusNoContent)
			//			return nil
		}
		id := target
		w, err := b.backend.PushBlobChunked(ctx, repo, id)
		if err != nil {
			return errTODO()
		}

		resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-3]...), "blobs/uploads", id))
		resp.Header().Set("Range", fmt.Sprintf("0-%d", w.Size()))
		resp.WriteHeader(http.StatusNoContent)
		return nil

	case http.MethodPut:
		if service != "uploads" {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "METHOD_UNKNOWN",
				Message: fmt.Sprintf("PUT to /blobs must be followed by /uploads, got %s", service),
			}
		}

		if digest == "" {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "DIGEST_INVALID",
				Message: "digest not specified",
			}
		}

		location := target
		w, err := b.backend.PushBlobChunked(ctx, repo, location)
		if err != nil {
			return errTODOf("%v; location %q; url %v", err, location, req.URL)
		}
		defer w.Close()

		_, err = hasher.NewHash(string(digest))
		if err != nil {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "NAME_INVALID",
				Message: "invalid digest",
			}
		}
		if _, err := io.Copy(w, req.Body); err != nil {
			return errTODO()
		}
		digest, err := w.Commit(ctx, digest)
		if err != nil {
			return errTODO()
		}
		resp.Header().Set("Docker-Content-Digest", string(digest))
		resp.WriteHeader(http.StatusCreated)
		return nil

	case http.MethodDelete:
		_, err := hasher.NewHash(target)
		if err != nil {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "NAME_INVALID",
				Message: "invalid digest",
			}
		}
		if err := b.backend.DeleteBlob(ctx, repo, ociregistry.Digest(target)); err != nil {
			return errTODO()
		}
		resp.WriteHeader(http.StatusAccepted)
		return nil

	default:
		return &regError{
			Status:  http.StatusBadRequest,
			Code:    "METHOD_UNKNOWN",
			Message: "We don't understand your method + url",
		}
	}
}
