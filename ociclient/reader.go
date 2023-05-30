package ociclient

import (
	"context"
	"fmt"

	"go.cuelabs.dev/ociregistry"
	"go.cuelabs.dev/ociregistry/internal/ocirequest"
)

func (c *client) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return c.read(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobGet,
		Repo:   repo,
		Digest: string(digest),
	})
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
	desc, err := descriptorFromResponse(resp, "", true)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	return desc, nil
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

func (c *client) read(ctx context.Context, rreq *ocirequest.Request) (_ ociregistry.BlobReader, _err error) {
	resp, err := c.doRequest(ctx, rreq)
	if err != nil {
		return nil, err
	}
	defer closeOnError(&_err, resp.Body)
	desc, err := descriptorFromResponse(resp, ociregistry.Digest(rreq.Digest), true)
	if err != nil {
		return nil, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	return &blobReader{
		ReadCloser: resp.Body,
		desc:       desc,
	}, nil
}
