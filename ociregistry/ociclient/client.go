// Copyright 2023 CUE Labs AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ociclient provides an implementation of ociregistry.Interface that
// uses HTTP to talk to the remote registry.
package ociclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
	"cuelabs.dev/go/oci/ociregistry/ociauth"
	"cuelabs.dev/go/oci/ociregistry/ociref"
)

// debug enables logging.
// TODO this should be configurable in the API.
const debug = false

type Options struct {
	// DebugID is used to prefix any log messages printed by the client.
	DebugID string

	// Transport is used to make HTTP requests. The context passed
	// to its RoundTrip method will have an appropriate
	// [ociauth.RequestInfo] value added, suitable for consumption
	// by the transport created by [ociauth.NewStdTransport]. If
	// Transport is nil, [http.DefaultTransport] will be used.
	//
	// Clients should use Transport to set a meaningful User-Agent header,
	// particularly when talking to registries they do not control;
	// see the userAgent example.
	Transport http.RoundTripper

	// Insecure specifies whether an http scheme will be used to
	// address the host instead of https.
	Insecure bool

	// ListPageSize configures the maximum number of results
	// requested when making list requests. If it's <= zero, it
	// defaults to DefaultListPageSize.
	ListPageSize int
}

// See https://github.com/google/go-containerregistry/issues/1091
// for an early report of the issue alluded to below.

// DefaultListPageSize holds the default number of results
// to request when using the list endpoints.
// It's not more than 1000 because AWS ECR complains
// it it's more than that.
const DefaultListPageSize = 1000

var debugID atomic.Int32

// New returns a registry implementation that uses the OCI
// HTTP API. A nil opts parameter is equivalent to a pointer
// to zero Options.
//
// The host specifies the host name to talk to; it may
// optionally be a host:port pair.
func New(host string, opts0 *Options) (ociregistry.Interface, error) {
	var opts Options
	if opts0 != nil {
		opts = *opts0
	}
	if opts.DebugID == "" {
		opts.DebugID = fmt.Sprintf("id%d", debugID.Add(1))
	}
	if opts.Transport == nil {
		opts.Transport = http.DefaultTransport
	}
	// Check that it's a valid host by forming a URL from it and checking that it matches.
	u, err := url.Parse("https://" + host + "/path")
	if err != nil {
		return nil, fmt.Errorf("invalid host %q", host)
	}
	if u.Host != host {
		return nil, fmt.Errorf("invalid host %q (does not correctly form a host part of a URL)", host)
	}
	if opts.Insecure {
		u.Scheme = "http"
	}
	if opts.ListPageSize == 0 {
		opts.ListPageSize = DefaultListPageSize
	}
	return &client{
		httpHost:   host,
		httpScheme: u.Scheme,
		httpClient: &http.Client{
			Transport: opts.Transport,
		},
		debugID:      opts.DebugID,
		listPageSize: opts.ListPageSize,
	}, nil
}

type client struct {
	*ociregistry.Funcs
	httpScheme   string
	httpHost     string
	httpClient   *http.Client
	debugID      string
	listPageSize int
}

type descriptorRequired byte

const (
	requireSize descriptorRequired = 1 << iota
	requireDigest
)

var (
	errUnknownContentLength = errors.New("unknown content length")
	errNoDigest             = errors.New("no digest found in response")
)

// descriptorFromResponse tries to form a descriptor from an HTTP response,
// filling in the Digest field using knownDigest if it's not present.
//
// Note: this implies that the Digest field will be empty if there is no
// digest in the response and knownDigest is empty.
func descriptorFromResponse(resp *http.Response, knownDigest digest.Digest, require descriptorRequired) (ociregistry.Descriptor, error) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	size := int64(0)
	if (require & requireSize) != 0 {
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
				return ociregistry.Descriptor{}, errUnknownContentLength
			}
			size = resp.ContentLength
		}
	}
	digest := digest.Digest(resp.Header.Get("Docker-Content-Digest"))
	if digest != "" {
		if !ociref.IsValidDigest(string(digest)) {
			return ociregistry.Descriptor{}, fmt.Errorf("bad digest %q found in response", digest)
		}
	} else {
		digest = knownDigest
	}
	if (require&requireDigest) != 0 && digest == "" {
		return ociregistry.Descriptor{}, errNoDigest
	}
	return ociregistry.Descriptor{
		Digest:    digest,
		MediaType: contentType,
		Size:      size,
	}, nil
}

func newBlobReader(r io.ReadCloser, desc ociregistry.Descriptor) *blobReader {
	return &blobReader{
		r:        r,
		digester: desc.Digest.Algorithm().Hash(),
		desc:     desc,
		verify:   true,
	}
}

func newBlobReaderUnverified(r io.ReadCloser, desc ociregistry.Descriptor) *blobReader {
	br := newBlobReader(r, desc)
	br.verify = false
	return br
}

type blobReader struct {
	r        io.ReadCloser
	n        int64
	digester hash.Hash
	desc     ociregistry.Descriptor
	verify   bool
}

func (r *blobReader) Descriptor() ociregistry.Descriptor {
	return r.desc
}

func (r *blobReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	r.n += int64(n)
	r.digester.Write(buf[:n])
	if err == nil {
		if r.n > r.desc.Size {
			// Fail early when the blob is too big; we can do that even
			// when we're not verifying for other use cases.
			return n, fmt.Errorf("blob size exceeds content length %d: %w", r.desc.Size, ociregistry.ErrSizeInvalid)
		}
		return n, nil
	}
	if err != io.EOF {
		return n, err
	}
	if !r.verify {
		return n, io.EOF
	}
	if r.n != r.desc.Size {
		return n, fmt.Errorf("blob size mismatch (%d/%d): %w", r.n, r.desc.Size, ociregistry.ErrSizeInvalid)
	}
	gotDigest := digest.NewDigest(r.desc.Digest.Algorithm(), r.digester)
	if gotDigest != r.desc.Digest {
		return n, fmt.Errorf("digest mismatch when reading blob")
	}
	return n, io.EOF
}

func (r *blobReader) Close() error {
	return r.r.Close()
}

// TODO make this list configurable.
var knownManifestMediaTypes = []string{
	ocispec.MediaTypeImageManifest,
	ocispec.MediaTypeImageIndex,
	"application/vnd.oci.artifact.manifest.v1+json", // deprecated.
	"application/vnd.docker.distribution.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	// Technically this wildcard should be sufficient, but it isn't
	// recognized by some registries.
	"*/*",
}

// doRequest performs the given OCI request, sending it with the given body (which may be nil).
func (c *client) doRequest(ctx context.Context, rreq *ocirequest.Request, okStatuses ...int) (*http.Response, error) {
	req, err := newRequest(ctx, rreq, nil)
	if err != nil {
		return nil, err
	}
	if rreq.Kind == ocirequest.ReqManifestGet || rreq.Kind == ocirequest.ReqManifestHead {
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
		req.URL.Scheme = c.httpScheme
	}
	if req.URL.Host == "" {
		req.URL.Host = c.httpHost
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
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot do HTTP request: %w", err)
	}
	// net/http only decompresses responses transparently when it asked
	// for gzip itself, so do the same when a server ignores our request
	// for identity encoding. Decompressing a partial response would not
	// work, as its range is over the compressed bytes.
	if resp.Header.Get("Content-Encoding") == "gzip" && resp.StatusCode != http.StatusPartialContent {
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
		resp.Body = &gzipReader{body: resp.Body}
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
	if len(okStatuses) == 0 && resp.StatusCode == http.StatusOK ||
		slices.Contains(okStatuses, resp.StatusCode) {
		// When asking for content as-is, any remaining encoding means
		// that neither the body nor the Content-Length match the content.
		if req.Header.Get("Accept-Encoding") == "identity" {
			if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
				resp.Body.Close()
				return nil, fmt.Errorf("unexpected Content-Encoding %q", enc)
			}
		}
		return resp, nil
	}
	defer resp.Body.Close()
	if !isOKStatus(resp.StatusCode) {
		return nil, makeError(resp)
	}
	return nil, unexpectedStatusError(resp.StatusCode)
}

// gzipReader decompresses a response body lazily, like net/http does,
// as the body may be empty or never read.
type gzipReader struct {
	body io.ReadCloser
	zr   *gzip.Reader
	err  error
}

func (r *gzipReader) Read(p []byte) (int, error) {
	if r.zr == nil && r.err == nil {
		r.zr, r.err = gzip.NewReader(r.body)
	}
	if r.err != nil {
		return 0, r.err
	}
	return r.zr.Read(p)
}

func (r *gzipReader) Close() error {
	return r.body.Close()
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

func closeOnError(err *error, r io.Closer) {
	if *err != nil {
		r.Close()
	}
}

func unexpectedStatusError(code int) error {
	return fmt.Errorf("unexpected HTTP response code %d", code)
}

func scopeForRequest(r *ocirequest.Request) ociauth.Scope {
	switch r.Kind {
	case ocirequest.ReqPing:
		return ociauth.Scope{}
	case ocirequest.ReqBlobGet,
		ocirequest.ReqBlobHead,
		ocirequest.ReqManifestGet,
		ocirequest.ReqManifestHead,
		ocirequest.ReqTagsList,
		ocirequest.ReqReferrersList:
		return ociauth.NewScope(ociauth.ResourceScope{
			ResourceType: ociauth.TypeRepository,
			Resource:     r.Repo,
			Action:       ociauth.ActionPull,
		})
	case ocirequest.ReqBlobDelete,
		ocirequest.ReqBlobStartUpload,
		ocirequest.ReqBlobUploadBlob,
		ocirequest.ReqBlobUploadInfo,
		ocirequest.ReqBlobUploadChunk,
		ocirequest.ReqBlobCompleteUpload,
		ocirequest.ReqManifestPut,
		ocirequest.ReqManifestDelete:
		return ociauth.NewScope(ociauth.ResourceScope{
			ResourceType: ociauth.TypeRepository,
			Resource:     r.Repo,
			Action:       ociauth.ActionPush,
		})
	case ocirequest.ReqBlobMount:
		return ociauth.NewScope(ociauth.ResourceScope{
			ResourceType: ociauth.TypeRepository,
			Resource:     r.Repo,
			Action:       ociauth.ActionPush,
		}, ociauth.ResourceScope{
			ResourceType: ociauth.TypeRepository,
			Resource:     r.FromRepo,
			Action:       ociauth.ActionPull,
		})
	case ocirequest.ReqCatalogList:
		return ociauth.NewScope(ociauth.CatalogScope)
	default:
		panic(fmt.Errorf("unexpected request kind %v", r.Kind))
	}
}

func newRequest(ctx context.Context, rreq *ocirequest.Request, body io.Reader) (*http.Request, error) {
	method, u, err := rreq.Construct()
	if err != nil {
		return nil, err
	}
	ctx = ociauth.ContextWithRequestInfo(ctx, ociauth.RequestInfo{
		RequiredScope: scopeForRequest(rreq),
	})
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	switch rreq.Kind {
	case ocirequest.ReqBlobGet, ocirequest.ReqBlobHead,
		ocirequest.ReqManifestGet, ocirequest.ReqManifestHead:
		// Ask for content as-is: compression gains little for blobs,
		// which are usually compressed already, nor for manifests,
		// which are usually small. Moreover, net/http's transparent
		// decompression would drop the Content-Length that we use
		// to determine the size of the content.
		req.Header.Set("Accept-Encoding", "identity")
	}
	return req, nil
}
