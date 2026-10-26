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

package ociclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocirequest"
	"github.com/opencontainers/go-digest"
)

func (c *client) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return c.read(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobGet,
		Repo:   repo,
		Digest: string(digest),
	})
}

func (c *client) GetBlobRange(ctx context.Context, repo string, digest ociregistry.Digest, o0, o1 int64) (_ ociregistry.BlobReader, _err error) {
	if o0 == 0 && o1 < 0 {
		return c.GetBlob(ctx, repo, digest)
	}
	rreq := &ocirequest.Request{
		Kind:   ocirequest.ReqBlobGet,
		Repo:   repo,
		Digest: string(digest),
	}
	req, err := newRequest(ctx, rreq, nil)
	if err != nil {
		return nil, err
	}
	if o1 < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", o0))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", o0, o1-1))
	}
	resp, err := c.do(req, http.StatusOK, http.StatusPartialContent)
	if err != nil {
		return nil, err
	}
	// TODO this is wrong when the server returns a 200 response.
	// Fix that either by returning ErrUnsupported or by reading the whole
	// blob and returning only the required portion.
	// TODO the returned reader is not verified either: it only fails
	// when reading more than the size of the whole blob, so a server
	// which responds with the wrong number of bytes for the range
	// goes unnoticed. We should check the range in Content-Range
	// as well as the number of bytes read.
	defer closeOnError(&_err, resp.Body)
	desc, err := descriptorFromResponse(resp, ociregistry.Digest(rreq.Digest), requireSize)
	if err != nil {
		return nil, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	return newBlobReaderUnverified(resp.Body, desc), nil
}

func (c *client) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return c.resolve(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobHead,
		Repo:   repo,
		Digest: string(digest),
	})
}

func (c *client) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return c.resolve(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqManifestHead,
		Repo:   repo,
		Digest: string(digest),
	})
}

func (c *client) ResolveTag(ctx context.Context, repo string, tag string) (ociregistry.Descriptor, error) {
	return c.resolve(ctx, &ocirequest.Request{
		Kind: ocirequest.ReqManifestHead,
		Repo: repo,
		Tag:  tag,
	})
}

func (c *client) resolve(ctx context.Context, rreq *ocirequest.Request) (ociregistry.Descriptor, error) {
	resp, err := c.doRequest(ctx, rreq)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	resp.Body.Close()
	desc, err := descriptorFromResponse(resp, ociregistry.Digest(rreq.Digest), 0)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	desc.Size = resp.ContentLength
	if desc.Size < 0 {
		if isEmptyDigest(desc.Digest) {
			// A ranged GET for empty content is unsatisfiable,
			// but we already know its size.
			desc.Size = 0
			return desc, nil
		}
		// Content-Length is not guaranteed in HEAD responses,
		// so fall back to a GET request to determine the size.
		// GET by the digest in this response if any,
		// as the GET response for a tag might lack a digest.
		return c.resolveWithGet(ctx, byDigest(rreq, desc.Digest))
	}
	if desc.Digest == "" {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor in response: %w", errNoDigest)
	}
	return desc, nil
}

// isEmptyDigest reports whether dig is the digest of empty content.
func isEmptyDigest(dig ociregistry.Digest) bool {
	return dig.Validate() == nil && dig == dig.Algorithm().FromBytes(nil)
}

// resolveWithGet resolves a HEAD request via a GET request instead.
// Blobs can be large, so for those it only asks for the first byte,
// as the response's Content-Range includes the size of the whole blob.
// Manifests are small, so they can be read into memory if needed.
func (c *client) resolveWithGet(ctx context.Context, rreq *ocirequest.Request) (ociregistry.Descriptor, error) {
	if rreq.Kind == ocirequest.ReqBlobHead {
		r, err := c.GetBlobRange(ctx, rreq.Repo, ociregistry.Digest(rreq.Digest), 0, 1)
		if err != nil {
			return ociregistry.Descriptor{}, err
		}
		drainAndClose(r, 2)
		return r.Descriptor(), nil
	}
	rreq.Kind = ocirequest.ReqManifestGet
	r, desc, err := c.get(ctx, rreq)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	drainAndClose(r, inMemThreshold)
	return desc, nil
}

// drainAndClose reads up to n bytes from r before closing it,
// so that its connection can be reused if that reaches the end of the body.
func drainAndClose(r io.ReadCloser, n int64) {
	io.Copy(io.Discard, io.LimitReader(r, n))
	r.Close()
}

func (c *client) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return c.read(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqManifestGet,
		Repo:   repo,
		Digest: string(digest),
	})
}

func (c *client) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	return c.read(ctx, &ocirequest.Request{
		Kind: ocirequest.ReqManifestGet,
		Repo: repo,
		Tag:  tagName,
	})
}

// inMemThreshold holds the maximum number of bytes of manifest content
// that we'll hold in memory to obtain its size or digest before falling
// back to doing a HEAD request.
//
// This is hopefully large enough to be considerably larger than most
// manifests but small enough to fit comfortably into RAM on most
// platforms.
//
// Note: this is only used when talking to registries that fail to return
// a Content-Length, or a digest when doing a GET on a tag.
const inMemThreshold = 128 * 1024

// get issues a GET request and returns a reader for its content.
// If the response lacks the size or digest of the content,
// get reads manifests of a reasonable size into memory to determine them;
// failing that, it closes the response and returns the partial descriptor
// along with an error wrapping [errUnknownContentLength] or [errNoDigest].
func (c *client) get(ctx context.Context, rreq *ocirequest.Request) (_ ociregistry.BlobReader, _ ociregistry.Descriptor, _err error) {
	resp, err := c.doRequest(ctx, rreq)
	if err != nil {
		return nil, ociregistry.Descriptor{}, err
	}
	defer closeOnError(&_err, resp.Body)
	desc, err := descriptorFromResponse(resp, ociregistry.Digest(rreq.Digest), 0)
	if err != nil {
		return nil, ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	// The size is unknown when the response has no Content-Length,
	// which HTTP does not guarantee; for example, with chunked encoding.
	desc.Size = resp.ContentLength
	if desc.Size >= 0 && desc.Digest != "" {
		return newBlobReader(resp.Body, desc), desc, nil
	}
	// Returning a digest isn't mandatory according to the spec, and
	// at least one registry (AWS's ECR) fails to return a digest
	// when doing a GET of a tag.
	// We know the request must be a tag-getting
	// request because all other requests take a digest not a tag
	// but sanity check anyway.
	isManifest := rreq.Kind == ocirequest.ReqManifestGet
	if desc.Digest == "" && !isManifest {
		return nil, ociregistry.Descriptor{}, fmt.Errorf("internal error: no digest available for non-tag request")
	}
	if isManifest && (desc.Size < 0 || desc.Size <= inMemThreshold) {
		// If the manifest is of a reasonable size, just read it into memory
		// and calculate the size and digest that way.
		data, err := io.ReadAll(io.LimitReader(resp.Body, inMemThreshold+1))
		if err != nil {
			return nil, ociregistry.Descriptor{}, fmt.Errorf("failed to read body to determine size and digest: %v", err)
		}
		if desc.Size >= 0 && int64(len(data)) != desc.Size {
			return nil, ociregistry.Descriptor{}, fmt.Errorf("body size mismatch")
		}
		if len(data) <= inMemThreshold {
			desc.Size = int64(len(data))
			if desc.Digest == "" {
				desc.Digest = digest.FromBytes(data)
			} else if desc.Digest != desc.Digest.Algorithm().FromBytes(data) {
				// Verify the content now rather than at EOF,
				// as the size depends on it.
				return nil, ociregistry.Descriptor{}, fmt.Errorf("body digest mismatch")
			}
			resp.Body.Close()
			return newBlobReader(io.NopCloser(bytes.NewReader(data)), desc), desc, nil
		}
	}
	if desc.Size < 0 {
		return nil, desc, fmt.Errorf("invalid descriptor in response: %w", errUnknownContentLength)
	}
	return nil, desc, fmt.Errorf("invalid descriptor in response: %w", errNoDigest)
}

// read issues a GET request, falling back to a HEAD request
// when [client.get] cannot determine the size or digest of the content.
func (c *client) read(ctx context.Context, rreq *ocirequest.Request) (ociregistry.BlobReader, error) {
	r, desc, err := c.get(ctx, rreq)
	if !errors.Is(err, errUnknownContentLength) && !errors.Is(err, errNoDigest) {
		return r, err
	}
	// Issue a HEAD request which should hopefully
	// (and does in the ECR case) give us what we need,
	// and then GET the content again by its digest,
	// which ensures that the content matches the HEAD response
	// even if the tag is updated between the requests.
	hreq := byDigest(rreq, desc.Digest)
	if rreq.Kind == ocirequest.ReqManifestGet {
		hreq.Kind = ocirequest.ReqManifestHead
	} else {
		hreq.Kind = ocirequest.ReqBlobHead
	}
	hdesc, err := c.resolve(ctx, hreq)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, byDigest(rreq, hdesc.Digest))
	if err != nil {
		return nil, err
	}
	return newBlobReader(resp.Body, hdesc), nil
}

// byDigest returns a copy of rreq which asks for the given digest
// rather than a tag, unless the digest is empty.
func byDigest(rreq *ocirequest.Request, dig ociregistry.Digest) *ocirequest.Request {
	rreq1 := *rreq
	if dig != "" {
		rreq1.Tag, rreq1.Digest = "", string(dig)
	}
	return &rreq1
}
