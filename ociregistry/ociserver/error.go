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

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Detail  any    `json:"detail,omitempty"`
}

type wireErrors struct {
	Errors []wireError `json:"errors"`
}

func writeError(resp http.ResponseWriter, err error) {
	e := wireError{
		Message: err.Error(),
	}
	var ociErr ociregistry.Error
	if errors.As(err, &ociErr) {
		e.Code = ociErr.Code()
		e.Detail = ociErr.Detail()
	} else {
		// This is contrary to spec, but it's what the Docker registry
		// does, so it can't be too bad.
		e.Code = "UNKNOWN"
	}
	httpStatus := http.StatusInternalServerError
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		httpStatus = statusErr.status
	} else if status, ok := errorStatuses[e.Code]; ok {
		httpStatus = status
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(httpStatus)

	data, err := json.Marshal(wireErrors{
		Errors: []wireError{e},
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

func badAPIUseError(f string, a ...any) error {
	return ociregistry.NewError(fmt.Sprintf(f, a...), ociregistry.ErrUnsupported.Code(), nil)
}

func withHTTPCode(status int, err error) error {
	if err == nil {
		panic("expected error to wrap")
	}
	return &httpStatusError{
		err:    err,
		status: status,
	}
}

type httpStatusError struct {
	err    error
	status int
}

func (e *httpStatusError) Unwrap() error {
	return e.err
}

func (e *httpStatusError) Error() string {
	return e.err.Error()
}
