package ocimem

import (
	"context"

	"cuelabs.dev/go/oci/ociregistry"
)

func (r *Registry) DeleteBlob(ctx context.Context, repoName string, digest ociregistry.Digest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.blobForDigest(repoName, digest); err != nil {
		return err
	}
	delete(r.repos[repoName].blobs, digest)
	return nil
}

func (r *Registry) DeleteManifest(ctx context.Context, repoName string, digest ociregistry.Digest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.manifestForDigest(repoName, digest); err != nil {
		return err
	}
	// TODO should this also delete any tags referring to this digest?
	delete(r.repos[repoName].manifests, digest)
	return nil
}

func (r *Registry) DeleteTag(ctx context.Context, repoName string, tagName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return err
	}
	desc, ok := repo.tags[tagName]
	if !ok {
		return ociregistry.ErrManifestUnknown
	}
	delete(repo.manifests, desc.Digest)
	delete(repo.tags, tagName)

	return nil
}
