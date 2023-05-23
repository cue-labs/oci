// Package ociclient provides an implementation of ociregistry.Interface that
// uses HTTP to talk to the remote registry.
package ociclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
	"github.com/rogpeppe/ociregistry/ocifunc"
)

func New(hostURL string) ociregistry.Interface {
	u, err := url.Parse(hostURL)
	if err != nil {
		panic(err)
	}
	return &client{
		url:       u,
		client:    http.DefaultClient,
		Interface: ocifunc.New(ocifunc.Funcs{}),
	}
}

type client struct {
	url    *url.URL
	client *http.Client
	ociregistry.Interface
}

func descriptorFromResponse(resp *http.Response) (ociregistry.Descriptor, error) {
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return ociregistry.Descriptor{}, fmt.Errorf("no digest found in response")
	}
	if !isValidDigest(digest) {
		return ociregistry.Descriptor{}, fmt.Errorf("bad digest %q found in response", digest)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if resp.ContentLength < 0 {
		return ociregistry.Descriptor{}, fmt.Errorf("unknown content length")
	}
	return ociregistry.Descriptor{
		Digest:    ociregistry.Digest(digest),
		MediaType: contentType,
		Size:      resp.ContentLength,
	}, nil
}

type blobReader struct {
	io.ReadCloser
	desc ociregistry.Descriptor
}

func (r *blobReader) Descriptor() ociregistry.Descriptor {
	return r.desc
}

func isValidDigest(d string) bool {
	_, err := digest.Parse(d)
	return err == nil
}

// doRequest performs the given OCI request, sending it with the given body (which may be nil).
func (c *client) doRequest(ctx context.Context, rreq *ocirequest.Request, body io.Reader, okStatuses ...int) (*http.Response, error) {
	method, u := rreq.Construct()
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, okStatuses...)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, makeError(resp)
}

func (c *client) do(req *http.Request, okStatuses ...int) (*http.Response, error) {
	if req.URL.Scheme == "" {
		req.URL.Scheme = c.url.Scheme
	}
	if req.URL.Host == "" {
		req.URL.Host = c.url.Host
	}
	if req.Body != nil {
		// Ensure that the body isn't consumed until the
		// server has responded that it will receive it.
		// This means that we can retry requests even when we've
		// got a consume-once-only io.Reader, such as
		// when pushing blobs.
		req.Header.Set("Expect", "100-continue")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot make HTTP request: %w", err)
	}
	for _, status := range okStatuses {
		if resp.StatusCode == status {
			return resp, nil
		}
	}
	defer resp.Body.Close()
	if !isOKStatus(resp.StatusCode) {
		return nil, makeError(resp)
	}
	return nil, unexpectedStatusError(resp.StatusCode)
}

func locationFromResponse(resp *http.Response) (*url.URL, error) {
	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("no Location found in response")
	}
	u, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("invalid Location URL found in response")
	}
	return resp.Request.URL.ResolveReference(u), nil
}

func isOKStatus(code int) bool {
	return code/100 == 2
}

func rangeString(x0, x1 int64) string {
	x1--
	if x1 < 0 {
		x1 = 0
	}
	return fmt.Sprintf("%d-%d", x0, x1)
}

func parseRange(s string) (int64, int64, bool) {
	p0s, p1s, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, false
	}
	p0, err0 := strconv.ParseInt(p0s, 10, 64)
	p1, err1 := strconv.ParseInt(p1s, 10, 64)
	if p1 > 0 {
		p1++
	}
	return p0, p1, err0 == nil && err1 == nil
}

func closeOnError(err *error, r io.Closer) {
	if *err != nil {
		r.Close()
	}
}

func unexpectedStatusError(code int) error {
	return fmt.Errorf("unexpected HTTP response code %d", code)
}
