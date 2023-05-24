package ociclient

import (
	"context"
	"net/http"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
)

func (c *client) DeleteBlob(ctx context.Context, repoName string, digest ociregistry.Digest) error {
	return c.delete(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqBlobDelete,
		Repo:   repoName,
		Digest: string(digest),
	})
}

func (c *client) DeleteManifest(ctx context.Context, repoName string, digest ociregistry.Digest) error {
	return c.delete(ctx, &ocirequest.Request{
		Kind:   ocirequest.ReqManifestDelete,
		Repo:   repoName,
		Digest: string(digest),
	})
}

func (c *client) DeleteTag(ctx context.Context, repoName string, tagName string) error {
	return c.delete(ctx, &ocirequest.Request{
		Kind: ocirequest.ReqManifestDelete,
		Repo: repoName,
		Tag:  tagName,
	})
}

func (c *client) delete(ctx context.Context, rreq *ocirequest.Request) error {
	resp, err := c.doRequest(ctx, rreq, http.StatusAccepted)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
