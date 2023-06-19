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
	"net/http"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

func (r *registry) handleBlobDelete(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	if err := r.backend.DeleteBlob(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest)); err != nil {
		return err
	}
	resp.WriteHeader(http.StatusAccepted)
	return nil
}

func (r *registry) handleManifestDelete(ctx context.Context, resp http.ResponseWriter, req *http.Request, rreq *ocirequest.Request) error {
	var err error
	if rreq.Tag != "" {
		err = r.backend.DeleteTag(ctx, rreq.Repo, rreq.Tag)
	} else {
		err = r.backend.DeleteManifest(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest))
	}
	if err != nil {
		return err
	}
	resp.WriteHeader(http.StatusAccepted)
	return nil
}
