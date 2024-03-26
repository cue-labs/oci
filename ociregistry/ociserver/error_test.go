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

package ociserver

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"

	"github.com/go-quicktest/qt"
)

func TestHTTPStatusOverriddenByErrorCode(t *testing.T) {
	// Test that if an Interface method returns an HTTPError error, the
	// HTTP status code is derived from the OCI error code in preference
	// to the HTTPError status code.
	r := New(&ociregistry.Funcs{
		GetTag_: func(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
			return nil, ociregistry.NewHTTPError(ociregistry.ErrNameUnknown, http.StatusUnauthorized, nil, nil)
		},
	}, nil)
	s := httptest.NewServer(r)
	defer s.Close()
	resp, err := http.Get(s.URL + "/v2/foo/manifests/sometag")
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusNotFound))
	qt.Assert(t, qt.JSONEquals(body, &ociregistry.WireErrors{
		Errors: []ociregistry.WireError{{
			Code_:   ociregistry.ErrNameUnknown.Code(),
			Message: "401 Unauthorized: repository name not known to registry",
		}},
	}))
}

func TestHTTPStatusUsedForUnknownErrorCode(t *testing.T) {
	// Test that if an Interface method returns an HTTPError error, that
	// HTTP status code is used when the code isn't known to be
	// associated with a particular HTTP status.
	r := New(&ociregistry.Funcs{
		GetTag_: func(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
			return nil, ociregistry.NewHTTPError(ociregistry.NewError("foo", "SOMECODE", nil), http.StatusTeapot, nil, nil)
		},
	}, nil)
	s := httptest.NewServer(r)
	defer s.Close()
	resp, err := http.Get(s.URL + "/v2/foo/manifests/sometag")
	qt.Assert(t, qt.IsNil(err))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	qt.Assert(t, qt.Equals(resp.StatusCode, http.StatusTeapot))
	qt.Assert(t, qt.JSONEquals(body, &ociregistry.WireErrors{
		Errors: []ociregistry.WireError{{
			Code_:   "SOMECODE",
			Message: "418 I'm a teapot: foo",
		}},
	}))
}
