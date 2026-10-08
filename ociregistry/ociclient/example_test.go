// Copyright 2026 CUE Labs AG
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

package ociclient_test

import (
	"log"
	"net/http"

	"cuelabs.dev/go/oci/ociregistry/ociauth"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
)

// userAgentTransport sets the User-Agent header on all requests.
type userAgentTransport struct {
	userAgent string
	transport http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the request it is given.
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.userAgent)
	return t.transport.RoundTrip(req)
}

// This example sets a User-Agent header on all requests made by the client,
// including those made to obtain authorization tokens.
func Example_userAgent() {
	config, err := ociauth.Load(nil)
	if err != nil {
		log.Fatal(err)
	}
	// The User-Agent transport is wrapped by the auth transport,
	// so that requests to auth servers also carry the header.
	transport := ociauth.NewStdTransport(ociauth.StdTransportParams{
		Config: config,
		Transport: &userAgentTransport{
			userAgent: "example-client/v1.2.3",
			transport: http.DefaultTransport,
		},
	})
	client, err := ociclient.New("registry.example.com", &ociclient.Options{
		Transport: transport,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = client // Use the client.
}
