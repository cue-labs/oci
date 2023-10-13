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

package ociserver_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
)

// Test that implementing an OCI registry proxy by sitting ociserver
// in front of ociclient doesn't introduce extra HTTP requests to the proxy backend.
//
// Each test case begins with a backend registry (ociserver in front of ocimem)
// with an HTTP middleware to record backend requests as they come in.
//
// We then set up a proxy (ociserver in front of ociclient) where the client points at the backend,
// and the server has a similar middleware to record the proxy requests as they come in.
//
// Finally, we have an ociclient pointing at the proxy which performs an OCI action via clientDo.
// We expect the proxy and backend requests to be practically the same
// as long as ociserver and ociclient do the right thing.

// ociclient defaults to a chunk size of 64KiB.
// We want our small data to fit in a single chunk,
// and large data to need at least three chunks to properly test PATCH edge cases.
var (
	smallData = bytes.Repeat([]byte("x"), 10)       // 10 B
	largeData = bytes.Repeat([]byte("x"), 150*1024) // 150 KiB
)

var proxyTests = []struct {
	name     string
	clientDo func(context.Context, ociregistry.Interface) error

	proxyRequests   []string
	backendRequests []string
}{
	{
		name: "PushBlob_small",
		clientDo: func(ctx context.Context, client ociregistry.Interface) error {
			_, err := client.PushBlob(ctx, "foo/bar", ocispec.Descriptor{
				MediaType: "application/octet-stream",
				Size:      int64(len(smallData)),
				Digest:    digest.FromBytes(smallData),
			}, bytes.NewReader(smallData))
			return err
		},
		proxyRequests: []string{
			"POST len=0",
			"PUT len=10",
		},
		backendRequests: []string{
			"POST len=0",
			"GET len=0",
			"PATCH len=10",
			"PUT len=0",
		},
	},
	{
		name: "PushBlob_large",
		clientDo: func(ctx context.Context, client ociregistry.Interface) error {
			_, err := client.PushBlob(ctx, "foo/bar", ocispec.Descriptor{
				MediaType: "application/octet-stream",
				Size:      int64(len(largeData)),
				Digest:    digest.FromBytes(largeData),
			}, bytes.NewReader(largeData))
			return err
		},
		proxyRequests: []string{
			"POST len=0",
			"PUT len=153600",
		},
		backendRequests: []string{
			"POST len=0",
			"GET len=0",
			"PATCH len=65536",
			"PATCH len=65536",
			"PATCH len=22528",
			"PUT len=0",
		},
	},
	{
		name: "PushBlobChunked_large_oneWrite",
		clientDo: func(ctx context.Context, client ociregistry.Interface) error {
			bw, err := client.PushBlobChunked(ctx, "foo/bar", 0)
			if err != nil {
				return err
			}
			if _, err := bw.Write(largeData); err != nil {
				return err
			}
			if _, err := bw.Commit(digest.FromBytes(largeData)); err != nil {
				return err
			}
			return nil
		},
		proxyRequests: []string{
			"POST len=0",
			"PATCH len=153600",
			"PUT len=0",
		},
		backendRequests: []string{
			"POST len=0",
			"GET len=0",
			"PATCH len=65536",
			"PATCH len=65536",
			"PATCH len=22528",
			"GET len=0",
			"PUT len=0",
		},
	},
}

func recordingServer(tb testing.TB, reqs *[]string, handler http.Handler) *httptest.Server {
	recHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reqs = append(*reqs, fmt.Sprintf("%s len=%d", r.Method, r.ContentLength))
		handler.ServeHTTP(w, r)
	})
	server := httptest.NewServer(recHandler)
	tb.Cleanup(server.Close)
	return server
}

func testClient(tb testing.TB, server *httptest.Server) ociregistry.Interface {
	client, err := ociclient.New(server.Listener.Addr().String(), &ociclient.Options{
		Insecure: true, // since it's a local httptest server
	})
	qt.Assert(tb, qt.IsNil(err))
	return client
}

func TestProxyRequests(t *testing.T) {
	for _, test := range proxyTests {
		t.Run(test.name, func(t *testing.T) {
			// Set up the backend (ociserver + ocimem)
			var proxyReqs, backendReqs []string
			backendServer := recordingServer(t, &backendReqs,
				ociserver.New(ocimem.New(), nil))

			// Set up the proxy (ociserver + ociclient).
			proxyServer := recordingServer(t, &proxyReqs,
				ociserver.New(testClient(t, backendServer), nil))

			// Set up the input client, mimicking the end user like cmd/cue.
			inputClient := testClient(t, proxyServer)

			// Run the input client action, and compare the results.
			err := test.clientDo(context.TODO(), inputClient)
			qt.Assert(t, qt.IsNil(err))

			qt.Check(t, qt.DeepEquals(proxyReqs, test.proxyRequests))
			qt.Check(t, qt.DeepEquals(backendReqs, test.backendRequests))
		})
	}
}
