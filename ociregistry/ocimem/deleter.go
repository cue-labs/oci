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
	if _, ok := repo.tags[tagName]; !ok {
		return ociregistry.ErrManifestUnknown
	}
	delete(repo.tags, tagName)

	return nil
}
