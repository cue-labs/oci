package ociclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociauth"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
)

func TestAuthScopes(t *testing.T) {

	// Test that we're passing the expected authorization scopes to the various parts of the API.
	// All the call semantics themselves are tested elsewhere, but we want to be
	// sure that we're passing the right required auth scopes to the authorizer.

	srv := httptest.NewServer(ociserver.New(ocimem.New(), nil))
	defer srv.Close()
	srvURL, _ := url.Parse(srv.URL)

	assertScope := func(scope string, f func(ctx context.Context, r ociregistry.Interface)) {
		assertAuthScope(t, srvURL.Host, scope, f)
	}

	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.GetBlob(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.GetBlobRange(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 100, 200)
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.GetManifest(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.GetTag(ctx, "foo/bar", "sometag")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.ResolveBlob(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.ResolveManifest(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.ResolveTag(ctx, "foo/bar", "sometag")
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		r.PushBlob(ctx, "foo/bar", ociregistry.Descriptor{
			MediaType: "application/json",
			Digest:    "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			Size:      3,
		}, strings.NewReader("foo"))
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		w, err := r.PushBlobChunked(ctx, "foo/bar", 0)
		qt.Assert(t, qt.IsNil(err))
		w.Write([]byte("foo"))
		w.Close()

		id := w.ID()
		w, err = r.PushBlobChunkedResume(ctx, "foo/bar", id, 3, 0)
		qt.Assert(t, qt.IsNil(err))
		w.Write([]byte("bar"))
		_, err = w.Commit(digest.FromString("foobar"))
		qt.Assert(t, qt.IsNil(err))
	})
	assertScope("repository:x/y:pull repository:z/w:push", func(ctx context.Context, r ociregistry.Interface) {
		r.MountBlob(ctx, "x/y", "z/w", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		r.PushManifest(ctx, "foo/bar", "sometag", []byte("something"), "application/json")
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		r.DeleteBlob(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		r.DeleteManifest(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	})
	assertScope("repository:foo/bar:push", func(ctx context.Context, r ociregistry.Interface) {
		r.DeleteTag(ctx, "foo/bar", "sometag")
	})
	assertScope("registry:catalog:*", func(ctx context.Context, r ociregistry.Interface) {
		ociregistry.All(r.Repositories(ctx, ""))
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		ociregistry.All(r.Tags(ctx, "foo/bar", ""))
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		ociregistry.All(r.Referrers(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ""))
	})
}

// assertAuthScope asserts that the given function makes a client request with the
// given scope to the given URL.
func assertAuthScope(t *testing.T, host string, scope string, f func(ctx context.Context, r ociregistry.Interface)) {
	requestedScopes := make(map[string]bool)

	// Check that the context is passed through with values intact.
	type foo struct{}
	ctx := context.WithValue(context.Background(), foo{}, true)

	client, err := New(host, &Options{
		Insecure: true,
		Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()
			qt.Check(t, qt.Equals(ctx.Value(foo{}), true))
			scope := ociauth.RequestInfoFromContext(ctx).RequiredScope
			requestedScopes[scope.Canonical().String()] = true
			return http.DefaultTransport.RoundTrip(req)
		}),
	})
	qt.Assert(t, qt.IsNil(err))
	f(ctx, client)
	qt.Assert(t, qt.HasLen(requestedScopes, 1))
	t.Logf("requested scopes: %v", requestedScopes)
	qt.Assert(t, qt.Equals(mapsKeys(requestedScopes)[0], scope))
}

type transportFunc func(req *http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TODO: replace with maps.Keys once Go adds it
func mapsKeys[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}
