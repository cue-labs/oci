package ociclient

import (
	"context"
	"fmt"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
)

func (c *client) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (_ ociregistry.BlobReader, _err error) {
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobGet,
		Repo:   repo,
		Digest: string(digest),
	}, nil)
	if err != nil {
		return nil, err
	}
	defer closeOnError(&_err, resp.Body)
	desc, err := descriptorFromResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	return &blobReader{
		ReadCloser: resp.Body,
		desc:       desc,
	}, nil
}

func (c *client) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobHead,
		Repo:   repo,
		Digest: string(digest),
	}, nil)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	resp.Body.Close()
	desc, err := descriptorFromResponse(resp)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor in response: %v", err)
	}
	return desc, nil
}
