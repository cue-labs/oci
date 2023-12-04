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
		Insecure:   true,
		HTTPClient: noDoer{},
	})
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.PanicMatches(func() {
		r.GetBlob(ctx, "Invalid--Repo", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	}, "invalid request.*: invalid repository name"))
	qt.Assert(t, qt.PanicMatches(func() {
		r.GetBlob(ctx, "okrepo", "bad-digest")
	}, "invalid request.*: badly formed digest"))
	qt.Assert(t, qt.PanicMatches(func() {
		r.ResolveTag(ctx, "okrepo", "bad-Tag!")
	}, "invalid request.*: page not found"))
}

type noDoer struct{}

func (noDoer) Do(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no can do")
}
