package ociclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/internal/ocirequest"
)

func (c *client) Repositories(ctx context.Context) ociregistry.Iter[string] {
	return errIter[string]{fmt.Errorf("Repositories unsupported: %w", ociregistry.ErrUnsupported)}
}

func (c *client) Tags(ctx context.Context, repoName string) ociregistry.Iter[string] {
	resp, err := c.doRequest(ctx, &ocirequest.Request{
		Kind:  ocirequest.ReqTagsList,
		Repo:  repoName,
		ListN: 10000,
	}, nil)
	if err != nil {
		return errIter[string]{err}
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return errIter[string]{err}
	}
	var tagsResponse struct {
		Repo string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(data, &tagsResponse); err != nil {
		return errIter[string]{fmt.Errorf("cannot unmarshal tags list response: %v", err)}
	}
	// TODO paging
	return ociregistry.SliceIter(tagsResponse.Tags)
}

func (c *client) Referrers(ctx context.Context, repoName string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	return errIter[ociregistry.Descriptor]{fmt.Errorf("Referrers unsupported: %w", ociregistry.ErrUnsupported)}
}

type errIter[T any] struct {
	err error
}

func (it errIter[T]) Close() {}

func (it errIter[T]) Next() (T, bool) {
	return *new(T), false
}

func (it errIter[T]) Error() error {
	return it.err
}
