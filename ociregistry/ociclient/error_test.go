package ociclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
	"github.com/go-quicktest/qt"
)

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
	// TODO(go1.23) for method, call := range ocitest.MethodCalls() {
	ocitest.MethodCalls()(func(method ocitest.Method, call ocitest.MethodCall) bool {
		t.Run(method.String(), func(t *testing.T) {
			err := call(context.Background(), r)
			t.Logf("call error: %v", err)
			var herr ociregistry.HTTPError
			ok := errors.As(err, &herr)
			qt.Assert(t, qt.IsTrue(ok))
			qt.Assert(t, qt.Equals(herr.StatusCode(), http.StatusTeapot))
		})
		return true
	})
}
