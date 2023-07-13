// Package ociclient provides an implementation of ociregistry.Interface that
// uses HTTP to talk to the remote registry.
package ociclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
)

// debug enables logging.
// TODO this should be configurable in the API.
const debug = false

type Options struct {
	// DebugID is used to prefix any log messages printed by the client.
	DebugID string

	// Client is used to send HTTP requests. If it's nil,
	// http.DefaultClient will be used.
	Client HTTPDoer
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var debugID int32

// New returns a registry implementation that uses the OCI
// HTTP API. A nil opts parameter is equivalent to a pointer
// to zero Options.
func New(hostURL string, opts *Options) ociregistry.Interface {
	var opts1 Options
	if opts != nil {
		opts1 = *opts
	}
	if opts1.DebugID == "" {
		opts1.DebugID = fmt.Sprintf("id%d", atomic.AddInt32(&debugID, 1))
	}
	if opts1.Client == nil {
		opts1.Client = http.DefaultClient
	}
	u, err := url.Parse(hostURL)
	if err != nil {
		panic(err)
	}
	return &client{
		url:     u,
		client:  opts1.Client,
		debugID: opts1.DebugID,
	}
}

type client struct {
	*ociregistry.Funcs
	url     *url.URL
	client  HTTPDoer
	debugID string
}

func descriptorFromResponse(resp *http.Response, knownDigest digest.Digest, requireSize bool) (ociregistry.Descriptor, error) {
	digest := digest.Digest(resp.Header.Get("Docker-Content-Digest"))
	if digest != "" {
		if !isValidDigest(string(digest)) {
			return ociregistry.Descriptor{}, fmt.Errorf("bad digest %q found in response", digest)
		}
	} else {
		if knownDigest == "" {
			return ociregistry.Descriptor{}, fmt.Errorf("no digest found in response")
		}
		digest = knownDigest
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	size := int64(0)
	if requireSize {
		if resp.StatusCode == http.StatusPartialContent {
			contentRange := resp.Header.Get("Content-Range")
			if contentRange == "" {
				return ociregistry.Descriptor{}, fmt.Errorf("no Content-Range in partial content response")
			}
			i := strings.LastIndex(contentRange, "/")
			if i == -1 {
				return ociregistry.Descriptor{}, fmt.Errorf("malformed Content-Range %q", contentRange)
			}
			contentSize, err := strconv.ParseInt(contentRange[i+1:], 10, 64)
			if err != nil {
				return ociregistry.Descriptor{}, fmt.Errorf("malformed Content-Range %q", contentRange)
			}
			size = contentSize
		} else {
			if resp.ContentLength < 0 {
				return ociregistry.Descriptor{}, fmt.Errorf("unknown content length")
			}
			size = resp.ContentLength
		}
	}
	return ociregistry.Descriptor{
		Digest:    digest,
		MediaType: contentType,
		Size:      size,
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

// TODO make this list configurable.
var knownManifestMediaTypes = []string{
	ocispec.MediaTypeImageManifest,
	ocispec.MediaTypeImageIndex,
	ocispec.MediaTypeArtifactManifest,
	"application/vnd.docker.container.image.v1+json",
	"application/vnd.docker.distribution.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	// Technically this wildcard should be sufficient, but it isn't
	// recognized by some registries.
	"*/*",
}

// doRequest performs the given OCI request, sending it with the given body (which may be nil).
func (c *client) doRequest(ctx context.Context, rreq *ocirequest.Request, okStatuses ...int) (*http.Response, error) {
	method, u := rreq.Construct()
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	if rreq.Kind == ocirequest.ReqManifestGet {
		// When getting manifests, some servers won't return
		// the content unless there's an Accept header, so
		// add all the manifest kinds that we know about.
		req.Header["Accept"] = knownManifestMediaTypes
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
	var buf bytes.Buffer
	if debug {
		fmt.Fprintf(&buf, "client.Do: %s %s {{\n", req.Method, req.URL)
		fmt.Fprintf(&buf, "\tBODY: %#v\n", req.Body)
		for k, v := range req.Header {
			fmt.Fprintf(&buf, "\t%s: %q\n", k, v)
		}
		c.logf("%s", buf.Bytes())
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot do HTTP request: %w", err)
	}
	if debug {
		buf.Reset()
		fmt.Fprintf(&buf, "} -> %s {\n", resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(&buf, "\t%s: %q\n", k, v)
		}
		data, _ := io.ReadAll(resp.Body)
		if len(data) > 0 {
			fmt.Fprintf(&buf, "\tBODY: %q\n", data)
		}
		fmt.Fprintf(&buf, "}}\n")
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(data))
		c.logf("%s", buf.Bytes())
	}
	if len(okStatuses) == 0 && resp.StatusCode == http.StatusOK {
		return resp, nil
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

func (c *client) logf(f string, a ...any) {
	log.Printf("ociclient %s: %s", c.debugID, fmt.Sprintf(f, a...))
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
