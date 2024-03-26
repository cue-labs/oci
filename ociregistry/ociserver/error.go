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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"cuelabs.dev/go/oci/ociregistry"
)

func writeError(resp http.ResponseWriter, err error) {
	e := ociregistry.WireError{
		Message: err.Error(),
	}
	// TODO perhaps we should iterate through all the
	// errors instead of just choosing one.
	// See https://github.com/golang/go/issues/66455
	var ociErr ociregistry.Error
	if errors.As(err, &ociErr) {
		e.Code_ = ociErr.Code()
		if detail := ociErr.Detail(); detail != nil {
			data, err := json.Marshal(detail)
			if err != nil {
				panic(fmt.Errorf("cannot marshal error detail: %v", err))
			}
			e.Detail_ = json.RawMessage(data)
		}
	} else {
		// This is contrary to spec, but it's what the Docker registry
		// does, so it can't be too bad.
		e.Code_ = "UNKNOWN"
	}

	// Use the HTTP status code from the error only when there isn't
	// one implied from the error code. This means that the HTTP status
	// is always consistent with the error code, but still allows a registry
	// to return custom HTTP status codes for other codes.
	httpStatus := http.StatusInternalServerError
	if status, ok := errorStatuses[e.Code_]; ok {
		httpStatus = status
	} else {
		var httpErr ociregistry.HTTPError
		if errors.As(err, &httpErr) {
			httpStatus = httpErr.StatusCode()
		}
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(httpStatus)

	data, err := json.Marshal(ociregistry.WireErrors{
		Errors: []ociregistry.WireError{e},
	})
	if err != nil {
		// TODO log
	}
	resp.Write(data)
}

var errorStatuses = map[string]int{
	ociregistry.ErrBlobUnknown.Code():         http.StatusNotFound,
	ociregistry.ErrBlobUploadInvalid.Code():   http.StatusRequestedRangeNotSatisfiable,
	ociregistry.ErrBlobUploadUnknown.Code():   http.StatusNotFound,
	ociregistry.ErrDigestInvalid.Code():       http.StatusBadRequest,
	ociregistry.ErrManifestBlobUnknown.Code(): http.StatusNotFound,
	ociregistry.ErrManifestInvalid.Code():     http.StatusBadRequest,
	ociregistry.ErrManifestUnknown.Code():     http.StatusNotFound,
	ociregistry.ErrNameInvalid.Code():         http.StatusBadRequest,
	ociregistry.ErrNameUnknown.Code():         http.StatusNotFound,
	ociregistry.ErrSizeInvalid.Code():         http.StatusBadRequest,
	ociregistry.ErrUnauthorized.Code():        http.StatusUnauthorized,
	ociregistry.ErrDenied.Code():              http.StatusForbidden,
	ociregistry.ErrUnsupported.Code():         http.StatusBadRequest,
	ociregistry.ErrTooManyRequests.Code():     http.StatusTooManyRequests,
	ociregistry.ErrRangeInvalid.Code():        http.StatusRequestedRangeNotSatisfiable,
}

func withHTTPCode(statusCode int, err error) error {
	return ociregistry.NewHTTPError(err, statusCode, nil, nil)
}

func badAPIUseError(f string, a ...any) error {
	return ociregistry.NewError(fmt.Sprintf(f, a...), ociregistry.ErrUnsupported.Code(), nil)
}
