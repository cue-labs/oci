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
	"iter"
	"slices"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
)

func (r *Registry) Repositories(_ context.Context, startAfter string) iter.Seq2[string, error] {
	r.mu.Lock()
	defer r.mu.Unlock()
	return mapKeysIter(r.repos, strings.Compare, startAfter)
}

func (r *Registry) Tags(_ context.Context, repoName string, startAfter string) iter.Seq2[string, error] {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return ociregistry.ErrorSeq[string](err)
	}
	return mapKeysIter(repo.tags, strings.Compare, startAfter)
}

func (r *Registry) Referrers(_ context.Context, repoName string, digest ociregistry.Digest, artifactType string) iter.Seq2[ociregistry.Descriptor, error] {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.repo(repoName)
	if err != nil {
		return ociregistry.ErrorSeq[ociregistry.Descriptor](err)
	}
	var referrers []ociregistry.Descriptor
	for _, b := range repo.manifests {
		if b.info.subject != digest {
			continue
		}
		if artifactType != "" && b.info.artifactType != artifactType {
			continue
		}
		referrers = append(referrers, b.descriptor())
	}
	slices.SortFunc(referrers, compareDescriptor)
	return ociregistry.SliceSeq(referrers)
}

func mapKeysIter[K comparable, V any](m map[K]V, cmp func(K, K) int, startAfter K) iter.Seq2[K, error] {
	ks := make([]K, 0, len(m))
	for k := range m {
		if cmp(startAfter, k) < 0 {
			ks = append(ks, k)
		}
	}
	slices.SortFunc(ks, cmp)

	return ociregistry.SliceSeq(ks)
}

func compareDescriptor(d0, d1 ociregistry.Descriptor) int {
	return strings.Compare(string(d0.Digest), string(d1.Digest))
}
