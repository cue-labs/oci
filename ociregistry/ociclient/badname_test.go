package ociclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestBadRepoName(t *testing.T) {
	ctx := context.Background()
	r, err := New("never.used", &Options{
		Insecure:  true,
		Transport: noTransport{},
	})
	qt.Assert(t, qt.IsNil(err))
	_, err = r.GetBlob(ctx, "Invalid--Repo", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	qt.Check(t, qt.ErrorMatches(err, "invalid OCI request: name invalid: invalid repository name"))
	_, err = r.GetBlob(ctx, "okrepo", "bad-digest")
	qt.Check(t, qt.ErrorMatches(err, "invalid OCI request: digest invalid: badly formed digest"))
	_, err = r.ResolveTag(ctx, "okrepo", "bad-Tag!")
	qt.Check(t, qt.ErrorMatches(err, "invalid OCI request: 404 Not Found: page not found"))
}

type noTransport struct{}

func (noTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no can do")
}
