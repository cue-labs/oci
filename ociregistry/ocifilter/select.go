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

	"cuelabs.dev/go/oci/ociregistry"
)

// AccessKind
type AccessKind int

const (
	// [ociregistry.Reader] methods.
	AccessRead AccessKind = iota

	// [ociregistry.Writer] methods.
	AccessWrite

	// [ociregistry.Deleter] methods.
	AccessDelete

	// [ociregistry.Lister] methods.
	AccessList
)

// AccessChecker returns a wrapper for r that invokes check
// to check access before calling an underlying method. Only if check succeeds will
// the underlying method be called.
//
// The check function is invoked with the name of the repository being
// accessed (or "*" for Repositories), and the kind of access required.
// For some methods (e.g. Mount), check might be invoked more than
// once for a given repository.
//
// When invoking the Repositories method, check is invoked for each repository in
// the iteration - the repository will be omitted if check returns an error.
func AccessChecker(r ociregistry.Interface, check func(repoName string, access AccessKind) error) ociregistry.Interface {
	return &accessCheckerRegistry{
		check: check,
		r:     r,
	}
}

type accessCheckerRegistry struct {
	// Embed Funcs rather than the interface directly so that
	// if new methods are added and selectRegistry isn't updated,
	// we fall back to returning an error rather than passing through the method.
	*ociregistry.Funcs
	check func(repoName string, kind AccessKind) error
	r     ociregistry.Interface
}

// Select returns a wrapper for r that provides only
// repositories for which allow returns true.
//
// Requests for disallowed repositories will return ErrNameUnknown
// errors on read and ErrDenied on write.
func Select(r ociregistry.Interface, allow func(repoName string) bool) ociregistry.Interface {
	return AccessChecker(r, func(repoName string, access AccessKind) error {
		if allow(repoName) {
			return nil
		}
		if access == AccessWrite {
			return ociregistry.ErrDenied
		}
		if access == AccessList && repoName == "*" {
			return nil
		}
		return ociregistry.ErrNameUnknown
	})
}

func (r *accessCheckerRegistry) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return nil, err
	}
	return r.r.GetBlob(ctx, repo, digest)
}

func (r *accessCheckerRegistry) GetBlobRange(ctx context.Context, repo string, digest ociregistry.Digest, offset0, offset1 int64) (ociregistry.BlobReader, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return nil, err
	}
	return r.r.GetBlobRange(ctx, repo, digest, offset0, offset1)
}

func (r *accessCheckerRegistry) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return nil, err
	}
	return r.r.GetManifest(ctx, repo, digest)
}

func (r *accessCheckerRegistry) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return nil, err
	}
	return r.r.GetTag(ctx, repo, tagName)
}

func (r *accessCheckerRegistry) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.ResolveBlob(ctx, repo, digest)
}

func (r *accessCheckerRegistry) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.ResolveManifest(ctx, repo, digest)
}

func (r *accessCheckerRegistry) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	if err := r.check(repo, AccessRead); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.ResolveTag(ctx, repo, tagName)
}

func (r *accessCheckerRegistry) PushBlob(ctx context.Context, repo string, desc ociregistry.Descriptor, rd io.Reader) (ociregistry.Descriptor, error) {
	if err := r.check(repo, AccessWrite); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.PushBlob(ctx, repo, desc, rd)
}

func (r *accessCheckerRegistry) PushBlobChunked(ctx context.Context, repo string, chunkSize int) (ociregistry.BlobWriter, error) {
	if err := r.check(repo, AccessWrite); err != nil {
		return nil, err
	}
	return r.r.PushBlobChunked(ctx, repo, chunkSize)
}

func (r *accessCheckerRegistry) PushBlobChunkedResume(ctx context.Context, repo, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	if err := r.check(repo, AccessWrite); err != nil {
		return nil, err
	}
	return r.r.PushBlobChunkedResume(ctx, repo, id, offset, chunkSize)
}

func (r *accessCheckerRegistry) MountBlob(ctx context.Context, fromRepo, toRepo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	if err := r.check(fromRepo, AccessRead); err != nil {
		return ociregistry.Descriptor{}, err
	}
	if err := r.check(toRepo, AccessWrite); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.MountBlob(ctx, fromRepo, toRepo, digest)
}

func (r *accessCheckerRegistry) PushManifest(ctx context.Context, repo string, tag string, contents []byte, mediaType string) (ociregistry.Descriptor, error) {
	if err := r.check(repo, AccessWrite); err != nil {
		return ociregistry.Descriptor{}, err
	}
	return r.r.PushManifest(ctx, repo, tag, contents, mediaType)
}

func (r *accessCheckerRegistry) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if err := r.check(repo, AccessDelete); err != nil {
		return err
	}
	return r.r.DeleteBlob(ctx, repo, digest)
}

func (r *accessCheckerRegistry) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	if err := r.check(repo, AccessDelete); err != nil {
		return err
	}
	return r.r.DeleteManifest(ctx, repo, digest)
}

func (r *accessCheckerRegistry) DeleteTag(ctx context.Context, repo string, name string) error {
	if err := r.check(repo, AccessDelete); err != nil {
		return err
	}
	return r.r.DeleteTag(ctx, repo, name)
}

func (r *accessCheckerRegistry) Repositories(ctx context.Context, startAfter string) iter.Seq2[string, error] {
	if err := r.check("*", AccessList); err != nil {
		return ociregistry.ErrorSeq[string](err)
	}
	return func(yield func(string, error) bool) {
		for repo, err := range r.r.Repositories(ctx, startAfter) {
			if err != nil {
				yield("", err)
				break
			}
			if r.check(repo, AccessRead) != nil {
				continue
			}
			if !yield(repo, nil) {
				break
			}
		}
	}
}

func (r *accessCheckerRegistry) Tags(ctx context.Context, repo, startAfter string) iter.Seq2[string, error] {
	if err := r.check(repo, AccessList); err != nil {
		return ociregistry.ErrorSeq[string](err)
	}
	return r.r.Tags(ctx, repo, startAfter)
}

func (r *accessCheckerRegistry) Referrers(ctx context.Context, repo string, digest ociregistry.Digest, artifactType string) iter.Seq2[ociregistry.Descriptor, error] {
	if err := r.check(repo, AccessList); err != nil {
		return ociregistry.ErrorSeq[ociregistry.Descriptor](err)
	}
	return r.r.Referrers(ctx, repo, digest, artifactType)
}
