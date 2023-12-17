package ociclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/exp/maps"
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
		r.Repositories(ctx)
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.Tags(ctx, "foo/bar")
	})
	assertScope("repository:foo/bar:pull", func(ctx context.Context, r ociregistry.Interface) {
		r.Referrers(ctx, "foo/bar", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "")
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
		Authorizer: authorizerFunc(func(req *http.Request, scope ociauth.Scope) (*http.Response, error) {
			qt.Check(t, qt.Equals(req.Context().Value(foo{}), true))
			requestedScopes[scope.Canonical().String()] = true
			return http.DefaultClient.Do(req)
		}),
	})
	qt.Assert(t, qt.IsNil(err))
	f(ctx, client)
	qt.Assert(t, qt.HasLen(requestedScopes, 1))
	qt.Assert(t, qt.Equals(maps.Keys(requestedScopes)[0], scope))
}

type authorizerFunc func(req *http.Request, scope ociauth.Scope) (*http.Response, error)

func (f authorizerFunc) DoRequest(req *http.Request, scope ociauth.Scope) (*http.Response, error) {
	return f(req, scope)
}
