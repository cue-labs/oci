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
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
)

// TestMissingContentLength checks that the client copes with responses
// lacking a Content-Length header, which HTTP does not guarantee,
// as long as it can determine the size by other means.
func TestMissingContentLength(t *testing.T) {
	const repo = "foo/bar"
	backend := ocimem.New()
	reg := ocitest.NewRegistry(t, backend)

	blob := []byte(strings.Repeat("some compressible blob content\n", 100))
	blobDesc := reg.MustPushBlob(repo, blob)
	manifestWithAnnotation := func(annotation string) *ociregistry.Manifest {
		return &ociregistry.Manifest{
			Versioned:   specs.Versioned{SchemaVersion: 2},
			MediaType:   ocispec.MediaTypeImageManifest,
			Config:      blobDesc,
			Annotations: map[string]string{"a": annotation},
		}
	}
	manifest, manifestDesc := reg.MustPushManifest(repo, manifestWithAnnotation("x"), "small")
	// Large enough that the client does not read it all into memory.
	largeManifest, largeManifestDesc := reg.MustPushManifest(repo, manifestWithAnnotation(strings.Repeat("x", 200*1024)), "large")

	type result struct {
		desc ociregistry.Descriptor
		data []byte // nil when only resolving
	}
	readAll := func(r ociregistry.BlobReader, err error) (result, error) {
		if err != nil {
			return result{}, err
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		return result{r.Descriptor(), data}, err
	}
	resolved := func(desc ociregistry.Descriptor, err error) (result, error) {
		return result{desc: desc}, err
	}
	ops := []struct {
		name string
		do   func(context.Context, ociregistry.Interface) (result, error)
		want result
	}{{
		name: "GetBlob",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetBlob(ctx, repo, blobDesc.Digest))
		},
		want: result{blobDesc, blob},
	}, {
		name: "GetBlobRange",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetBlobRange(ctx, repo, blobDesc.Digest, 10, 100))
		},
		want: result{blobDesc, blob[10:100]},
	}, {
		name: "GetManifest",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetManifest(ctx, repo, manifestDesc.Digest))
		},
		want: result{manifestDesc, manifest},
	}, {
		name: "GetTag",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetTag(ctx, repo, "small"))
		},
		want: result{manifestDesc, manifest},
	}, {
		name: "GetTagLarge",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetTag(ctx, repo, "large"))
		},
		want: result{largeManifestDesc, largeManifest},
	}, {
		name: "ResolveBlob",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveBlob(ctx, repo, blobDesc.Digest))
		},
		want: result{desc: blobDesc},
	}, {
		name: "ResolveManifest",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveManifest(ctx, repo, manifestDesc.Digest))
		},
		want: result{desc: manifestDesc},
	}, {
		name: "ResolveTag",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveTag(ctx, repo, "small"))
		},
		want: result{desc: manifestDesc},
	}, {
		name: "ResolveTagLarge",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveTag(ctx, repo, "large"))
		},
		want: result{desc: largeManifestDesc},
	}}

	const errNoLength = `.*unknown content length`
	const errGzip = `.*unexpected Content-Encoding "gzip"`
	servers := []struct {
		name string
		wrap func(http.Handler) http.Handler
		// wantErrors maps operation names to the errors they should fail with;
		// all other operations should succeed.
		wantErrors map[string]string
	}{{
		name: "Plain",
		wrap: func(h http.Handler) http.Handler { return h },
	}, {
		name: "NoContentLengthOnGET",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"GET"}, "Content-Length")
		},
	}, {
		name: "NoContentLengthOnHEAD",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"HEAD"}, "Content-Length")
		},
		wantErrors: map[string]string{
			// These should fall back to GET requests, ranged for blobs.
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
			"ResolveTagLarge": errNoLength,
		},
	}, {
		name: "NoContentLength",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"GET", "HEAD"}, "Content-Length")
		},
		wantErrors: map[string]string{
			"GetTagLarge":     errNoLength,
			"ResolveTagLarge": errNoLength,
			// These should fall back to GET requests, ranged for blobs.
			"GetBlob":         errNoLength,
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
		},
	}, {
		name: "NoContentLengthOrDigest",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"GET", "HEAD"}, "Content-Length", "Docker-Content-Digest")
		},
		wantErrors: map[string]string{
			"GetTagLarge":     errNoLength,
			"ResolveTagLarge": errNoLength,
			// These should fall back to GET requests, ranged for blobs.
			"GetBlob":         errNoLength,
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
		},
	}, {
		name: "NoContentLengthOnHEADOrDigestOnGET",
		wrap: func(h http.Handler) http.Handler {
			h = withoutHeaders(h, []string{"HEAD"}, "Content-Length")
			return withoutHeaders(h, []string{"GET"}, "Docker-Content-Digest")
		},
		wantErrors: map[string]string{
			// These should fall back to GET requests, ranged for blobs,
			// and by the digest from the HEAD response for tags.
			"GetTagLarge":     errNoLength,
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
			"ResolveTagLarge": errNoLength,
		},
	}, {
		name: "GzipWhenAccepted",
		wrap: func(h http.Handler) http.Handler { return gzipped(h, false) },
	}, {
		name: "GzipAlways",
		wrap: func(h http.Handler) http.Handler { return gzipped(h, true) },
		wantErrors: map[string]string{
			"GetBlob":      errGzip,
			"GetBlobRange": errGzip,
			"GetManifest":  errGzip,
			"GetTag":       errGzip,
			"GetTagLarge":  errGzip,
		},
	}}
	for _, srv := range servers {
		t.Run(srv.name, func(t *testing.T) {
			hsrv := httptest.NewServer(srv.wrap(ociserver.New(backend, nil)))
			t.Cleanup(hsrv.Close)
			// Limit connections per host, so that the client blocks
			// if it holds a response open while making another request.
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.MaxConnsPerHost = 1
			client := mustNewOCIClient(hsrv.URL, &ociclient.Options{Transport: transport})
			for _, op := range ops {
				t.Run(op.name, func(t *testing.T) {
					got, err := op.do(t.Context(), client)
					if wantErr := srv.wantErrors[op.name]; wantErr != "" {
						qt.Assert(t, qt.ErrorMatches(err, wantErr))
						return
					}
					qt.Assert(t, qt.IsNil(err))
					qt.Assert(t, qt.DeepEquals(got.desc, op.want.desc))
					// Compare as strings, as qt.DeepEquals is slow on large byte slices.
					qt.Assert(t, qt.Equals(string(got.data), string(op.want.data)))
				})
			}
		})
	}
}

// withoutHeaders removes the given headers from h's responses to requests
// with any of the given methods. GET responses are flushed straight away,
// so that net/http uses chunked encoding rather than computing a Content-Length.
func withoutHeaders(h http.Handler, methods []string, headers ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !slices.Contains(methods, req.Method) {
			h.ServeHTTP(w, req)
			return
		}
		hw := &headerRemover{ResponseWriter: w, flush: req.Method == "GET", headers: headers}
		h.ServeHTTP(hw, req)
		hw.WriteHeader(http.StatusOK) // in case h wrote nothing
	})
}

type headerRemover struct {
	http.ResponseWriter
	flush       bool
	headers     []string
	wroteHeader bool
}

func (w *headerRemover) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	for _, h := range w.headers {
		w.Header().Del(h)
	}
	w.ResponseWriter.WriteHeader(code)
	if w.flush {
		w.ResponseWriter.(http.Flusher).Flush()
	}
}

func (w *headerRemover) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.ResponseWriter.Write(p)
}

// gzipped gzip-encodes h's responses to GET requests.
// Unless always is true, it only does so when the request accepts it.
func gzipped(h http.Handler, always bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" || !always && !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
			h.ServeHTTP(w, req)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		h.ServeHTTP(gw, req)
		if gw.gz != nil {
			gw.gz.Close()
		}
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (w *gzipWriter) WriteHeader(code int) {
	if w.gz != nil {
		return
	}
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Encoding", "gzip")
	w.ResponseWriter.WriteHeader(code)
	w.gz = gzip.NewWriter(w.ResponseWriter)
}

func (w *gzipWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.gz.Write(p)
}
