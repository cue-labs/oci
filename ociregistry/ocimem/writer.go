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

package ocimem

import (
	"context"
	"fmt"
	"io"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/opencontainers/go-digest"
)

// This file implements the ociregistry.Writer methods.

func (r *Registry) PushBlob(ctx context.Context, repoName string, desc ociregistry.Descriptor, content io.Reader) (ociregistry.Descriptor, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("cannot read content: %v", err)
	}
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
	return desc, nil
}

func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, chunkSize int) (ociregistry.BlobWriter, error) {
	// TODO(mvdan): Why does the ocimem implementation allow a PATCH on an upload ID which doesn't exist?
	// The tests in ociserver make this assumption, so they break without this bit of code.
	//
	// Ideally they should start a new chunked upload to get a new ID, then use that for PATCH/PUT.
	// Alternatively, add a new method to ocimem outside of the interface to start a chunked upload with a predefined ID.
	// Either way, this case should be an error, per the spec.
	return r.PushBlobChunkedResume(ctx, repoName, "", 0, chunkSize)
}

func (r *Registry) PushBlobChunkedResume(ctx context.Context, repoName, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	b := repo.uploads[id]
	if b == nil {
		b = NewBuffer(func(b *Buffer) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			desc, data, _ := b.GetBlob()
			repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
			return nil
		}, id)
		repo.uploads[b.ID()] = b
	}
	b.checkStartOffset = offset
	return b, nil
}

func (r *Registry) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rto, err := r.makeRepo(toRepo)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	b, err := r.blobForDigest(fromRepo, dig)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	rto.blobs[dig] = b
	return b.descriptor(), nil
}

var errCannotOverwriteTag = fmt.Errorf("%w: cannot overwrite tag", ociregistry.ErrDenied)

func (r *Registry) PushManifest(ctx context.Context, repoName string, tag string, data []byte, mediaType string) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	dig := digest.FromBytes(data)
	desc := ociregistry.Descriptor{
		Digest:    dig,
		MediaType: mediaType,
		Size:      int64(len(data)),
	}
	if tag != "" {
		if !ociregistry.IsValidTag(tag) {
			return ociregistry.Descriptor{}, fmt.Errorf("invalid tag")
		}
		if r.cfg.ImmutableTags {
			if currDesc, ok := repo.tags[tag]; ok {
				if dig == currDesc.Digest {
					if currDesc.MediaType != mediaType {
						// Same digest but mismatched media type.
						return ociregistry.Descriptor{}, fmt.Errorf("%w: mismatched media type", ociregistry.ErrDenied)
					}
					// It's got the same content already. Allow it.
					return currDesc, nil
				}
				return ociregistry.Descriptor{}, errCannotOverwriteTag
			}
		}
	}
	// make a copy of the data to avoid potential corruption.
	data = append([]byte(nil), data...)
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}
	subject, err := r.checkManifest(repoName, desc.MediaType, data)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid manifest: %v", err)
	}

	repo.manifests[dig] = &blob{
		mediaType: mediaType,
		data:      data,
		subject:   subject,
	}
	if tag != "" {
		repo.tags[tag] = desc
	}
	return desc, nil
}

func (r *Registry) checkManifest(repoName string, mediaType string, data []byte) (subject ociregistry.Digest, retErr error) {
	repo, err := r.repo(repoName)
	if err != nil {
		return "", err
	}
	iter, err := manifestReferences(mediaType, data)
	if err != nil {
		// TODO decide what to do about errUnknownManifestMediaTypeForIteration
		return "", err
	}
	iter(func(info descInfo) bool {
		if err := CheckDescriptor(info.desc, nil); err != nil {
			retErr = fmt.Errorf("bad descriptor in %s: %v", info.name, err)
			return false
		}
		switch info.kind {
		case kindBlob:
			if repo.blobs[info.desc.Digest] == nil {
				retErr = fmt.Errorf("blob for %s not found", info.name)
				return false
			}
		case kindManifest:
			if repo.manifests[info.desc.Digest] == nil {
				retErr = fmt.Errorf("manifest for %s not found", info.name)
				return false
			}
		case kindSubjectManifest:
			subject = info.desc.Digest
			// The standard explicitly specifies that we can have
			// a dangling subject so don't check that it exists.
		}
		return true
	})
	return subject, retErr
}

// refersTo reports whether the given digest is referred to, directly or indirectly, by any item
// returned by the given iterator, within the given repository.
// TODO currently this iterates through all tagged manifests. A better
// algorithm could amortise that work and be considerably more efficient.
func refersTo(repo *repository, iter descIter, digest ociregistry.Digest) (found bool, retErr error) {
	iter(func(info descInfo) bool {
		if info.desc.Digest == digest {
			found = true
			return false
		}
		switch info.kind {
		case kindManifest, kindSubjectManifest:
			b := repo.manifests[info.desc.Digest]
			if b == nil {
				break
			}
			miter, err := manifestReferences(info.desc.MediaType, b.data)
			if err != nil {
				retErr = err
				return false
			}
			found, retErr = refersTo(repo, miter, digest)
			if found || retErr != nil {
				return false
			}
		}
		return true
	})
	return found, retErr
}
