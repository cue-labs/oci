package ociclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociauth"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
	"github.com/go-quicktest/qt"
)

func TestAuthScopes(t *testing.T) {

	// Test that we're passing the expected authorization scopes to the various parts of the API.
	// All the call semantics themselves are tested elsewhere, but we want to be
	// sure that we're passing the right required auth scopes to the authorizer.

	srv := httptest.NewServer(ociserver.New(ocimem.New(), nil))
	defer srv.Close()
	srvURL, _ := url.Parse(srv.URL)

	wantScopes := map[ocitest.Method]string{
		ocitest.GetBlob:               "repository:foo/read:pull",
		ocitest.GetBlobRange:          "repository:foo/read:pull",
		ocitest.GetManifest:           "repository:foo/read:pull",
		ocitest.GetTag:                "repository:foo/read:pull",
		ocitest.ResolveBlob:           "repository:foo/read:pull",
		ocitest.ResolveManifest:       "repository:foo/read:pull",
		ocitest.ResolveTag:            "repository:foo/read:pull",
		ocitest.PushBlob:              "repository:foo/write:push",
		ocitest.PushBlobChunked:       "repository:foo/write:push",
		ocitest.PushBlobChunkedResume: "repository:foo/write:push",
		ocitest.MountBlob:             "repository:foo/read:pull repository:foo/write:push",
		ocitest.PushManifest:          "repository:foo/write:push",
		ocitest.DeleteBlob:            "repository:foo/write:push",
		ocitest.DeleteManifest:        "repository:foo/write:push",
		ocitest.DeleteTag:             "repository:foo/write:push",
		ocitest.Repositories:          "registry:catalog:*",
		ocitest.Tags:                  "repository:foo/read:pull",
		ocitest.Referrers:             "repository:foo/read:pull",
	}
	// TODO(go1.23) for method, call := range ocitest.MethodCalls() {
	ocitest.MethodCalls()(func(method ocitest.Method, call ocitest.MethodCall) bool {
		t.Run(method.String(), func(t *testing.T) {
			assertAuthScope(t, srvURL.Host, wantScopes[method], func(ctx context.Context, r ociregistry.Interface) {
				err := call(ctx, r)
				t.Logf("call error: %v", err)
			})
		})
		return true
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
