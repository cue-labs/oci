package ocimem

import (
	"context"
	"fmt"

	"github.com/rogpeppe/ociregistry"
)

func (r *Registry) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return fmt.Errorf("DeleteBlob TODO")
}

func (r *Registry) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return fmt.Errorf("DeleteManifest TODO")
}
func (r *Registry) DeleteTag(ctx context.Context, repo string, name string) error {
	return fmt.Errorf("DeleteTag TODO")
}
