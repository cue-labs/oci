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

package ociunify

import (
	"context"

	"cuelabs.dev/go/oci/ociregistry"
)

// Deleter methods

// TODO all these methods should not raise an error if deleting succeeds in one
// registry but fails due to a not-found error in the other.

func (u unifier) DeleteBlob(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteBlob(ctx, repo, digest))
	})).err
}

func (u unifier) DeleteManifest(ctx context.Context, repo string, digest ociregistry.Digest) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteManifest(ctx, repo, digest))
	})).err
}

func (u unifier) DeleteTag(ctx context.Context, repo string, name string) error {
	return bothResults(both(u, func(r ociregistry.Interface, _ int) t1 {
		return mk1(r.DeleteTag(ctx, repo, name))
	})).err
}
