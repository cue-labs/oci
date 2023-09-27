package ocifilter

import (
	"context"
	"io"
	"path"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
)

// Sub returns r wrapped so that it addresses only
// repositories within pathPrefix.
//
// The prefix must match an entire path element so,
// for example, if the prefix is "foo", "foo" and "foo/a" will
// be included, but "foobie" will not.
//
// For example, if r has the following repositories:
//
//	a
//	a/b/c
//	a/d
//	x/p
//	aa/b
//
// then Sub(r "a") will return a registry containing the following repositories:
//
//	b/c
//	d
func Sub(r ociregistry.Interface, pathPrefix string) ociregistry.Interface {
	if pathPrefix == "" {
		return r
	}
	return &subRegistry{
		prefix: pathPrefix,
		r:      r,
	}
}

type subRegistry struct {
	*ociregistry.Funcs
	prefix string
	r      ociregistry.Interface
}

func (r *subRegistry) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return r.r.GetBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) GetBlobRange(ctx context.Context, repo string, digest ociregistry.Digest, offset0, offset1 int64) (ociregistry.BlobReader, error) {
	return r.r.GetBlobRange(ctx, r.repo(repo), digest, offset0, offset1)
}

func (r *subRegistry) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return r.r.GetManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	return r.r.GetTag(ctx, r.repo(repo), tagName)
}

func (r *subRegistry) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return r.r.ResolveBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return r.r.ResolveManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	return r.r.ResolveTag(ctx, r.repo(repo), tagName)
}

func (r *subRegistry) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, rd io.Reader) (ociregistry.Descriptor, error) {
	return r.r.PushBlob(ctx, r.repo(repo), desc, rd)
}

func (r *subRegistry) PushBlobChunked(ctx context.Context, repo string, id string, chunkSize int) (ociregistry.BlobWriter, error) {
	return r.r.PushBlobChunked(ctx, r.repo(repo), id, chunkSize)
}

func (r *subRegistry) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return r.r.MountBlob(ctx, r.repo(fromRepo), r.repo(toRepo), digest)
}

func (r *subRegistry) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	return r.r.PushManifest(ctx, r.repo(repo), tag, contents, mediaType)
}

func (r *subRegistry) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return r.r.DeleteBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return r.r.DeleteManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) DeleteTag(ctx context.Context, repo string, name string) error {
	return r.r.DeleteTag(ctx, r.repo(repo), name)
}

func (r *subRegistry) Repositories(ctx context.Context) ociregistry.Iter[string] {
	return &subRegistryIter{
		pr:   r,
		iter: r.r.Repositories(ctx),
	}
}

func (r *subRegistry) Tags(ctx context.Context, repo string) ociregistry.Iter[string] {
	return r.r.Tags(ctx, r.repo(repo))
}

func (r *subRegistry) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	return r.r.Referrers(ctx, r.repo(repo), digest, artifactType)
}

func (r *subRegistry) repo(name string) string {
	if name == "" {
		// An empty repository name isn't allowed, so keep it
		// like that so that the underlying registry will reject the
		// empty name.
		return ""
	}
	return path.Join(r.prefix, name)
}

type subRegistryIter struct {
	pr   *subRegistry
	iter ociregistry.Iter[string]
}

func (it *subRegistryIter) Close() {
	it.iter.Close()
}

func (it *subRegistryIter) Next() (string, bool) {
	p := it.pr.prefix + "/"
	for {
		x, ok := it.iter.Next()
		if !ok {
			return "", false
		}
		if p, ok := strings.CutPrefix(x, p); ok {
			return p, true
		}
	}
}

func (it *subRegistryIter) Error() error {
	return it.iter.Error()
}
