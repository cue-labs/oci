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

// Package ociserver implements a docker V2 registry and the OCI distribution specification.
//
// It is designed to be used anywhere a low dependency container registry is needed.
//
// Its goal is to be standards compliant and its strictness will increase over time.
package ociserver

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
	ocispecroot "github.com/opencontainers/image-spec/specs-go"
)

// debug causes debug messages to be emitted when running the server.
const debug = false

var v2 = ocispecroot.Versioned{
	SchemaVersion: 2,
}

// Options holds options for the server.
type Options struct {
	// WriteError is used to write error responses. It is passed the
	// writer to write the error response to, the request that
	// the error is in response to, and the error itself.
	//
	// If WriteError is nil, [ociregistry.WriteError] will
	// be used and any error discarded.
	WriteError func(w http.ResponseWriter, req *http.Request, err error)

	// DisableReferrersAPI, when true, causes the registry to behave as if
	// it does not understand the referrers API.
	DisableReferrersAPI bool

	// DisableReferrersFiltering, when true, cause the registry
	// to behave as if it does not recognize the artifactType filter
	// on the referrers API.
	DisableReferrersFiltering bool

	// DisableSinglePostUpload, when true, causes the registry
	// to reject uploads with a single POST request.
	// This is useful in combination with LocationsForDescriptor
	// to cause uploaded blob content to flow through
	// another server.
	DisableSinglePostUpload bool

	// MaxListPageSize, if > 0, causes the list endpoints to return an
	// error if the page size is greater than that. This emulates
	// a quirk of AWS ECR where it refuses request for any
	// page size > 1000.
	MaxListPageSize int

	// OmitDigestFromTagGetResponse causes the registry
	// to omit the Docker-Content-Digest header from a tag
	// GET response, mimicking the behavior of registries that
	// do the same (for example AWS ECR).
	OmitDigestFromTagGetResponse bool

	// OmitLinkHeaderFromResponses causes the server
	// to leave out the Link header from list responses.
	OmitLinkHeaderFromResponses bool

	// LocationForUploadID transforms an upload ID as returned by
	// ocirequest.BlobWriter.ID to the absolute URL location
	// as returned by the upload endpoints.
	//
	// By default, when this function is nil, or it returns an empty
	// string, upload IDs are treated as opaque identifiers and the
	// returned locations are always host-relative URLs into the
	// server itself.
	//
	// This can be used to allow clients to fetch and push content
	// directly from some upstream server rather than passing
	// through this server. Clients doing that will need access
	// rights to that remote location.
	LocationForUploadID func(string) (string, error)

	// LocationsForDescriptor returns a set of possible download
	// URLs for the given descriptor.
	// If it's nil, then all locations returned by the server
	// will refer to the server itself.
	//
	// If not, then the Location header of responses will be
	// set accordingly (to an arbitrary value from the
	// returned slice if there are multiple).
	//
	// Returning a location from this function will also
	// cause GET requests to return a redirect response
	// to that location.
	//
	// TODO perhaps the redirect behavior described above
	// isn't always what is wanted?
	LocationsForDescriptor func(isManifest bool, desc ociregistry.Descriptor) ([]string, error)

	DebugID string
}

var debugID int32

// New returns a handler which implements the docker registry protocol
// by making calls to the underlying registry backend r.
//
// If opts is nil, it's equivalent to passing new(Options).
//
// The returned handler should be registered at the site root.
//
// # Errors
//
// All HTTP responses will be JSON, formatted according to the
// OCI spec. If an error returned from backend conforms to
// [ociregistry.Error], the associated code and detail will be used.
//
// The HTTP response code will be determined from the error
// code when possible. If it can't be determined and the
// error implements [ociregistry.HTTPError], the code returned
// by StatusCode will be used as the HTTP response code.
func New(backend ociregistry.Interface, opts *Options) http.Handler {
	if opts == nil {
		opts = new(Options)
	}
	r := &registry{
		opts:    *opts,
		backend: backend,
	}
	if r.opts.DebugID == "" {
		r.opts.DebugID = fmt.Sprintf("ociserver%d", atomic.AddInt32(&debugID, 1))
	}
	if r.opts.WriteError == nil {
		r.opts.WriteError = func(w http.ResponseWriter, _ *http.Request, err error) {
			ociregistry.WriteError(w, err)
		}
	}
	return r
}

func (r *registry) logf(f string, a ...any) {
	log.Printf("ociserver %s: %s", r.opts.DebugID, fmt.Sprintf(f, a...))
}

type registry struct {
	opts    Options
	backend ociregistry.Interface
}

var handlers = []func(r *registry, ctx context.Context, w http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error{
	ocirequest.ReqPing:               (*registry).handlePing,
	ocirequest.ReqBlobGet:            (*registry).handleBlobGet,
	ocirequest.ReqBlobHead:           (*registry).handleBlobHead,
	ocirequest.ReqBlobDelete:         (*registry).handleBlobDelete,
	ocirequest.ReqBlobStartUpload:    (*registry).handleBlobStartUpload,
	ocirequest.ReqBlobUploadBlob:     (*registry).handleBlobUploadBlob,
	ocirequest.ReqBlobMount:          (*registry).handleBlobMount,
	ocirequest.ReqBlobUploadInfo:     (*registry).handleBlobUploadInfo,
	ocirequest.ReqBlobUploadChunk:    (*registry).handleBlobUploadChunk,
	ocirequest.ReqBlobCompleteUpload: (*registry).handleBlobCompleteUpload,
	ocirequest.ReqManifestGet:        (*registry).handleManifestGet,
	ocirequest.ReqManifestHead:       (*registry).handleManifestHead,
	ocirequest.ReqManifestPut:        (*registry).handleManifestPut,
	ocirequest.ReqManifestDelete:     (*registry).handleManifestDelete,
	ocirequest.ReqTagsList:           (*registry).handleTagsList,
	ocirequest.ReqReferrersList:      (*registry).handleReferrersList,
	ocirequest.ReqCatalogList:        (*registry).handleCatalogList,
}

func (r *registry) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if rerr := r.v2(resp, req); rerr != nil {
		r.opts.WriteError(resp, req, rerr)
		return
	}
}

// https://docs.docker.com/registry/spec/api/#api-version-check
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#api-version-check
func (r *registry) v2(resp http.ResponseWriter, req *http.Request) (_err error) {
	if debug {
		r.logf("registry.v2 %v %s {", req.Method, req.URL)
		defer func() {
			if _err != nil {
				r.logf("} -> %v", _err)
			} else {
				r.logf("}")
			}
		}()
	}

	rreq, err := ocirequest.Parse(req.Method, req.URL)
	if err != nil {
		resp.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		return err
	}
	handle := handlers[rreq.Kind]
	return handle(r, req.Context(), resp, req, rreq)
}

func (r *registry) handlePing(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	resp.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	return nil
}

func (r *registry) setLocationHeader(resp http.ResponseWriter, isManifest bool, desc ociregistry.Descriptor, defaultLocation string) error {
	loc := defaultLocation
	if r.opts.LocationsForDescriptor != nil {
		locs, err := r.opts.LocationsForDescriptor(isManifest, desc)
		if err != nil {
			what := "blob"
			if isManifest {
				what = "manifest"
			}
			return fmt.Errorf("cannot determine location for %s: %v", what, err)
		}
		if len(locs) > 0 {
			loc = locs[0] // TODO select arbitrary location from the slice
		}
	}
	resp.Header().Set("Location", loc)
	resp.Header().Set("Docker-Content-Digest", string(desc.Digest))
	return nil
}
