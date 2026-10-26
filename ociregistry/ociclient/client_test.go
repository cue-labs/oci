package ociclient_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocidebug"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
)

func TestReferrersFallback(t *testing.T) {
	ctx := t.Context()

	// Test that the client falls back to using the referrers tag API
	// when the referrers API is not enabled.
	srv := httptest.NewServer(ociserver.New(ocidebug.New(ocimem.New(), t.Logf), &ociserver.Options{
		DisableReferrersAPI: true,
	}))
	t.Cleanup(srv.Close)

	client := mustNewOCIClient(srv.URL, nil)

	const repo = "foo/bar"

	// Push a scratch config for all the manifests to refer to.
	config := pushScratchConfig(t, client, repo)

	// Push a manifest to refer to.
	subject := pushManifest(t, client, repo, "sometag", &ociregistry.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    withMediaType(config, "subject/mediatype"),
	}, ocispec.MediaTypeImageManifest)

	index := &ocispec.Index{
		MediaType: ocispec.MediaTypeImageIndex,
	}
	// Then push some manifests that refer to it and update the index at the same time.
	for i := range 5 {
		artifactType := fmt.Sprintf("referrer/%d", i)
		desc := pushManifest(t, client, repo, "", &ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Subject:   &subject,
			Config:    withMediaType(config, artifactType),
		}, ocispec.MediaTypeImageManifest)
		desc.ArtifactType = artifactType
		index.Manifests = append(index.Manifests, desc)
	}

	// Then push the index to the referrers tag.
	pushManifest(t, client, repo, strings.ReplaceAll(string(subject.Digest), ":", "-"), index, ocispec.MediaTypeImageIndex)

	// Then ask for the referrers.
	var got []ociregistry.Descriptor
	for desc, err := range client.Referrers(ctx, repo, subject.Digest, "") {
		qt.Assert(t, qt.IsNil(err))
		got = append(got, desc)
	}
	qt.Assert(t, qt.DeepEquals(got, index.Manifests))

	// Check that artifact type filtering still works OK.
	got = nil
	for desc, err := range client.Referrers(ctx, repo, subject.Digest, "referrer/2") {
		qt.Assert(t, qt.IsNil(err))
		got = append(got, desc)
	}
	qt.Assert(t, qt.DeepEquals(got, []ociregistry.Descriptor{index.Manifests[2]}))
}

func withMediaType(desc ociregistry.Descriptor, mediaType string) ociregistry.Descriptor {
	desc.MediaType = mediaType
	return desc
}

func pushScratchConfig(t *testing.T, client ociregistry.Interface, repo string) ociregistry.Descriptor {
	content := []byte("{}")
	desc := ocispec.Descriptor{
		Digest: digest.FromBytes(content),
		Size:   int64(len(content)),
	}
	_, err := client.PushBlob(t.Context(), repo, desc, bytes.NewReader(content))
	qt.Assert(t, qt.IsNil(err))
	return desc
}

func pushManifest(t *testing.T, client ociregistry.Interface, repo, tag string, content any, mediaType string) ociregistry.Descriptor {
	data, err := json.Marshal(content)
	qt.Assert(t, qt.IsNil(err))
	desc, err := client.PushManifest(t.Context(), repo, tag, data, mediaType)
	qt.Assert(t, qt.IsNil(err))
	return desc
}

func mustNewOCIClient(srvURL string, opts *ociclient.Options) ociregistry.Interface {
	if opts == nil {
		opts = new(ociclient.Options)
	}
	u, err := url.Parse(srvURL)
	if err != nil {
		panic(err)
	}
	opts.Insecure = u.Scheme == "http"
	client, err := ociclient.New(u.Host, opts)
	if err != nil {
		panic(err)
	}
	return client
}

// TestMissingContentLength checks that the client copes with responses
// lacking a Content-Length header, which HTTP does not guarantee,
// as long as it can determine the size by other means.
func TestMissingContentLength(t *testing.T) {
	const repo = "foo/bar"
	backend := ocimem.New()
	reg := ocitest.NewRegistry(t, backend)

	blob := []byte(strings.Repeat("some compressible blob content\n", 100))
	blobDesc := reg.MustPushBlob(repo, blob)
	emptyBlobDesc := reg.MustPushBlob(repo, nil)
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
		name: "GetEmptyBlob",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return readAll(r.GetBlob(ctx, repo, emptyBlobDesc.Digest))
		},
		want: result{emptyBlobDesc, []byte{}},
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
		// Error responses have bodies too, which must be decoded.
		name: "GetManifestUnknown",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			_, err := r.GetManifest(ctx, repo, digest.FromString("unknown"))
			if errors.Is(err, ociregistry.ErrManifestUnknown) {
				err = nil
			}
			return result{}, err
		},
	}, {
		name: "ResolveBlob",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveBlob(ctx, repo, blobDesc.Digest))
		},
		want: result{desc: blobDesc},
	}, {
		name: "ResolveEmptyBlob",
		do: func(ctx context.Context, r ociregistry.Interface) (result, error) {
			return resolved(r.ResolveBlob(ctx, repo, emptyBlobDesc.Digest))
		},
		want: result{desc: emptyBlobDesc},
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
	const errBlocked = `.*context deadline exceeded`
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
		wantErrors: map[string]string{
			// These should read manifests into memory,
			// or fall back to a HEAD request.
			"GetBlob":      errNoLength,
			"GetEmptyBlob": errNoLength,
			"GetManifest":  errNoLength,
			"GetTag":       errNoLength,
			"GetTagLarge":  errNoLength,
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
			// The digest of an empty blob gives its size away.
			"ResolveEmptyBlob": errNoLength,
		},
	}, {
		name: "NoContentLength",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"GET", "HEAD"}, "Content-Length")
		},
		wantErrors: map[string]string{
			"GetTagLarge":     errNoLength,
			"ResolveTagLarge": errNoLength,
			// These should read the manifests into memory.
			"GetManifest": errNoLength,
			"GetTag":      errNoLength,
			// These should fall back to GET requests, ranged for blobs.
			"GetBlob":         errNoLength,
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
			// The digest of an empty blob gives its size away.
			"GetEmptyBlob":     errNoLength,
			"ResolveEmptyBlob": errNoLength,
		},
	}, {
		name: "NoContentLengthOrDigest",
		wrap: func(h http.Handler) http.Handler {
			return withoutHeaders(h, []string{"GET", "HEAD"}, "Content-Length", "Docker-Content-Digest")
		},
		wantErrors: map[string]string{
			"GetTagLarge":     errNoLength,
			"ResolveTagLarge": errNoLength,
			// These should read the manifests into memory.
			"GetManifest": errNoLength,
			"GetTag":      errNoLength,
			// These should fall back to GET requests, ranged for blobs.
			"GetBlob":         errNoLength,
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
			// The digest of an empty blob gives its size away.
			"GetEmptyBlob":     errNoLength,
			"ResolveEmptyBlob": errNoLength,
		},
	}, {
		name: "NoContentLengthOnHEADOrDigestOnGET",
		wrap: func(h http.Handler) http.Handler {
			h = withoutHeaders(h, []string{"HEAD"}, "Content-Length")
			return withoutHeaders(h, []string{"GET"}, "Docker-Content-Digest")
		},
		wantErrors: map[string]string{
			// The client holds the GET response open while making
			// a HEAD request, which blocks with one connection per host.
			"GetTagLarge": errBlocked,
			// These should fall back to GET requests, ranged for blobs,
			// and by the digest from the HEAD response for tags.
			"ResolveBlob":     errNoLength,
			"ResolveManifest": errNoLength,
			"ResolveTag":      errNoLength,
			"ResolveTagLarge": errNoLength,
			// The digest of an empty blob gives its size away.
			"ResolveEmptyBlob": errNoLength,
		},
	}, {
		name: "GzipWhenAccepted",
		wrap: func(h http.Handler) http.Handler { return gzipped(h, false) },
	}, {
		name: "GzipAlways",
		wrap: func(h http.Handler) http.Handler { return gzipped(h, true) },
		wantErrors: map[string]string{
			// These are decompressed, dropping their Content-Length,
			// so they should read manifests into memory,
			// or fall back to a HEAD request.
			"GetBlob":      errNoLength,
			"GetEmptyBlob": errNoLength,
			"GetManifest":  errNoLength,
			"GetTag":       errNoLength,
			"GetTagLarge":  errNoLength,
			// The range is over the compressed bytes.
			"GetBlobRange": errGzip,
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
					// Some operations block; see errBlocked.
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					got, err := op.do(ctx, client)
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
