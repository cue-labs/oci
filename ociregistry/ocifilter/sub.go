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

package ocifilter

import (
	"context"
	"io"
	"iter"
	"path"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociauth"
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

// TODO adjust any auth scopes in the context as they pass through.

type subRegistry struct {
	*ociregistry.Funcs
	prefix string
	r      ociregistry.Interface
}

func (r *subRegistry) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	ctx = r.mapScopes(ctx)
	return r.r.GetBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) GetBlobRange(ctx context.Context, repo string, digest ociregistry.Digest, offset0, offset1 int64) (ociregistry.BlobReader, error) {
	ctx = r.mapScopes(ctx)
	return r.r.GetBlobRange(ctx, r.repo(repo), digest, offset0, offset1)
}

func (r *subRegistry) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	ctx = r.mapScopes(ctx)
	return r.r.GetManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	ctx = r.mapScopes(ctx)
	return r.r.GetTag(ctx, r.repo(repo), tagName)
}

func (r *subRegistry) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.ResolveBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.ResolveManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.ResolveTag(ctx, r.repo(repo), tagName)
}

func (r *subRegistry) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, rd io.Reader) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.PushBlob(ctx, r.repo(repo), desc, rd)
}

func (r *subRegistry) PushBlobChunked(ctx context.Context, repo string, chunkSize int) (ociregistry.BlobWriter, error) {
	ctx = r.mapScopes(ctx)
	// Luckily the context spans the entire lifetime of the blob writer (no
	// BlobWriter methods take a Context argument, so no need
	// to wrap it.
	return r.r.PushBlobChunked(ctx, r.repo(repo), chunkSize)
}

func (r *subRegistry) PushBlobChunkedResume(ctx context.Context, repo, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	ctx = r.mapScopes(ctx)
	return r.r.PushBlobChunkedResume(ctx, r.repo(repo), id, offset, chunkSize)
}

func (r *subRegistry) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.MountBlob(ctx, r.repo(fromRepo), r.repo(toRepo), digest)
}

func (r *subRegistry) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	ctx = r.mapScopes(ctx)
	return r.r.PushManifest(ctx, r.repo(repo), tag, contents, mediaType)
}

func (r *subRegistry) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	ctx = r.mapScopes(ctx)
	return r.r.DeleteBlob(ctx, r.repo(repo), digest)
}

func (r *subRegistry) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	ctx = r.mapScopes(ctx)
	return r.r.DeleteManifest(ctx, r.repo(repo), digest)
}

func (r *subRegistry) DeleteTag(ctx context.Context, repo string, name string) error {
	ctx = r.mapScopes(ctx)
	return r.r.DeleteTag(ctx, r.repo(repo), name)
}

func (r *subRegistry) Repositories(ctx context.Context, startAfter string) iter.Seq2[string, error] {
	ctx = r.mapScopes(ctx)
	p := r.prefix + "/"
	return func(yield func(string, error) bool) {
		for repo, err := range r.r.Repositories(ctx, startAfter) {
			if err != nil {
				yield("", err)
				break
			}
			if p, ok := strings.CutPrefix(repo, p); ok && !yield(p, nil) {
				break
			}
		}
	}
}

func (r *subRegistry) Tags(ctx context.Context, repo, startAfter string) iter.Seq2[string, error] {
	ctx = r.mapScopes(ctx)
	return r.r.Tags(ctx, r.repo(repo), startAfter)
}

func (r *subRegistry) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) iter.Seq2[ociregistry.Descriptor, error] {
	ctx = r.mapScopes(ctx)
	return r.r.Referrers(ctx, r.repo(repo), digest, artifactType)
}

// mapScopes changes any auth scopes in the context so that
// they refer to the prefixed names rather than the originals.
func (r *subRegistry) mapScopes(ctx context.Context) context.Context {
	scope := ociauth.ScopeFromContext(ctx)
	if scope.IsEmpty() {
		return ctx
	}
	// TODO we could potentially provide a Scope constructor
	// that took an iterator, which could avoid the intermediate
	// slice allocation.
	scopes := make([]ociauth.ResourceScope, 0, scope.Len())
	for rs := range scope.Iter() {
		if rs.ResourceType == ociauth.TypeRepository {
			rs.Resource = r.repo(rs.Resource)
		}
		scopes = append(scopes, rs)
	}
	return ociauth.ContextWithScope(ctx, ociauth.NewScope(scopes...))
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
