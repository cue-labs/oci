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

	"cuelabs.dev/go/oci/ociregistry"
)

// Select returns a wrapper for r that provides only
// repositories for which allow returns true.
//
// Requests for disallowed repositories will return ErrNameUnknown
// errors on read and ErrDenied on write.
func Select(r ociregistry.Interface, allow func(repoName string) bool) ociregistry.Interface {
	return &selectRegistry{
		allow: allow,
		r:     r,
	}
}

type selectRegistry struct {
	// Embed Funcs rather than the interface directly so that
	// if new methods are added and selectRegistry isn't updated,
	// we fall back to returning an error rather than passing through the method.
	*ociregistry.Funcs
	allow func(repoName string) bool
	r     ociregistry.Interface
}

func (r *selectRegistry) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrNameUnknown
	}
	return r.r.GetBlob(ctx, repo, digest)
}

func (r *selectRegistry) GetBlobRange(ctx context.Context, repo string, digest ociregistry.Digest, offset0, offset1 int64) (ociregistry.BlobReader, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrNameUnknown
	}
	return r.r.GetBlobRange(ctx, repo, digest, offset0, offset1)
}

func (r *selectRegistry) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrNameUnknown
	}
	return r.r.GetManifest(ctx, repo, digest)
}

func (r *selectRegistry) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrNameUnknown
	}
	return r.r.GetTag(ctx, repo, tagName)
}

func (r *selectRegistry) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if !r.allow(repo) {
		return ociregistry.Descriptor{}, ociregistry.ErrNameUnknown
	}
	return r.r.ResolveBlob(ctx, repo, digest)
}

func (r *selectRegistry) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if !r.allow(repo) {
		return ociregistry.Descriptor{}, ociregistry.ErrNameUnknown
	}
	return r.r.ResolveManifest(ctx, repo, digest)
}

func (r *selectRegistry) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	if !r.allow(repo) {
		return ociregistry.Descriptor{}, ociregistry.ErrNameUnknown
	}
	return r.r.ResolveTag(ctx, repo, tagName)
}

func (r *selectRegistry) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, rd io.Reader) (ociregistry.Descriptor, error) {
	if !r.allow(repo) {
		return ociregistry.Descriptor{}, ociregistry.ErrDenied
	}
	return r.r.PushBlob(ctx, repo, desc, rd)
}

func (r *selectRegistry) PushBlobChunked(ctx context.Context, repo string, chunkSize int) (ociregistry.BlobWriter, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrDenied
	}
	return r.r.PushBlobChunked(ctx, repo, chunkSize)
}

func (r *selectRegistry) PushBlobChunkedResume(ctx context.Context, repo, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	if !r.allow(repo) {
		return nil, ociregistry.ErrDenied
	}
	return r.r.PushBlobChunkedResume(ctx, repo, id, offset, chunkSize)
}

func (r *selectRegistry) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if !r.allow(toRepo) {
		return ociregistry.Descriptor{}, ociregistry.ErrDenied
	}
	if !r.allow(fromRepo) {
		return ociregistry.Descriptor{}, ociregistry.ErrNameUnknown
	}
	return r.r.MountBlob(ctx, fromRepo, toRepo, digest)
}

func (r *selectRegistry) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	if !r.allow(repo) {
		return ociregistry.Descriptor{}, ociregistry.ErrDenied
	}
	return r.r.PushManifest(ctx, repo, tag, contents, mediaType)
}

func (r *selectRegistry) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if !r.allow(repo) {
		return ociregistry.ErrNameUnknown
	}
	return r.r.DeleteBlob(ctx, repo, digest)
}

func (r *selectRegistry) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if !r.allow(repo) {
		return ociregistry.ErrNameUnknown
	}
	return r.r.DeleteManifest(ctx, repo, digest)
}

func (r *selectRegistry) DeleteTag(ctx context.Context, repo string, name string) error {
	if !r.allow(repo) {
		return ociregistry.ErrNameUnknown
	}
	return r.r.DeleteTag(ctx, repo, name)
}

func (r *selectRegistry) Repositories(ctx context.Context) ociregistry.Iter[string] {
	return &filterIter[string]{
		allow: r.allow,
		iter:  r.r.Repositories(ctx),
	}
}

func (r *selectRegistry) Tags(ctx context.Context, repo string) ociregistry.Iter[string] {
	if !r.allow(repo) {
		return ociregistry.ErrorIter[string](ociregistry.ErrNameUnknown)
	}
	return r.r.Tags(ctx, repo)
}

func (r *selectRegistry) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) ociregistry.Iter[ociregistry.Descriptor] {
	if !r.allow(repo) {
		return ociregistry.ErrorIter[ociregistry.Descriptor](ociregistry.ErrNameUnknown)
	}
	return r.r.Referrers(ctx, repo, digest, artifactType)
}

type filterIter[T any] struct {
	allow func(T) bool
	iter  ociregistry.Iter[T]
}

func (it *filterIter[T]) Close() {
	it.iter.Close()
}

func (it *filterIter[T]) Next() (T, bool) {
	for {
		x, ok := it.iter.Next()
		if !ok {
			return *new(T), false
		}
		if it.allow(x) {
			return x, true
		}
	}
}

func (it *filterIter[T]) Error() error {
	return it.iter.Error()
}
