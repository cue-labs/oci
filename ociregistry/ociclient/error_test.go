package ociclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
)

func TestErrorStuttering(t *testing.T) {
	// This checks that the stuttering observed in issue #31
	// isn't an issue when ociserver wraps ociclient.
	srv := httptest.NewServer(ociserver.New(&ociregistry.Funcs{
		NewError: func(ctx context.Context, methodName, repo string) error {
			return ociregistry.ErrManifestUnknown
		},
	}, nil))
	defer srv.Close()

	srvURL, _ := url.Parse(srv.URL)
	r, err := New(srvURL.Host, &Options{
		Insecure: true,
	})
	qt.Assert(t, qt.IsNil(err))
	_, err = r.GetTag(context.Background(), "foo", "sometag")
	qt.Check(t, qt.ErrorIs(err, ociregistry.ErrManifestUnknown))
	qt.Check(t, qt.ErrorMatches(err, `404 Not Found: manifest unknown: manifest unknown to registry`))

	// ResolveTag uses HEAD rather than GET, so here we're testing
	// the path where a response with no body gets turned back into
	// something vaguely resembling the original error, which is why
	// the code and message have changed.
	_, err = r.ResolveTag(context.Background(), "foo", "sometag")
	qt.Check(t, qt.ErrorIs(err, ociregistry.ErrNameUnknown))
	qt.Check(t, qt.ErrorMatches(err, `404 Not Found: name unknown: repository name not known to registry`))
}

func TestNonJSONErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("some body"))
	}))
	defer srv.Close()

	srvURL, _ := url.Parse(srv.URL)
	r, err := New(srvURL.Host, &Options{
		Insecure: true,
	})
	qt.Assert(t, qt.IsNil(err))
	assertStatusCode := func(f func(ctx context.Context, r ociregistry.Interface) error) {
		err := f(context.Background(), r)
		var herr ociregistry.HTTPError
		ok := errors.As(err, &herr)
		qt.Assert(t, qt.IsTrue(ok))
		qt.Assert(t, qt.Equals(herr.StatusCode(), http.StatusTeapot))
	}
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		if rd != nil {
			rd.Close()
		}
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetBlobRange(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 100, 200)
		if rd != nil {
			rd.Close()
		}
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		if rd != nil {
			rd.Close()
		}
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		rd, err := r.GetTag(ctx, "foo/read", "sometag")
		if rd != nil {
			rd.Close()
		}
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.ResolveTag(ctx, "foo/read", "sometag")
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.PushBlob(ctx, "foo/write", ociregistry.Descriptor{
			MediaType: "application/json",
			Digest:    "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			Size:      3,
		}, strings.NewReader("foo"))
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		w, err := r.PushBlobChunked(ctx, "foo/write", 0)
		if err != nil {
			return err
		}
		w.Close()
		return nil
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		w, err := r.PushBlobChunkedResume(ctx, "foo/write", "/someid", 3, 0)
		if err != nil {
			return err
		}
		data := []byte("some data")
		if _, err := w.Write(data); err != nil {
			return err
		}
		_, err = w.Commit(digest.FromBytes(data))
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.MountBlob(ctx, "foo/read", "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := r.PushManifest(ctx, "foo/write", "sometag", []byte("something"), "application/json")
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteBlob(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteManifest(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		return r.DeleteTag(ctx, "foo/write", "sometag")
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Repositories(ctx, ""))
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Tags(ctx, "foo/read", ""))
		return err
	})
	assertStatusCode(func(ctx context.Context, r ociregistry.Interface) error {
		_, err := ociregistry.All(r.Referrers(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ""))
		return err
	})
}
