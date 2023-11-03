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
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociauth"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
)

func TestSub(t *testing.T) {
	ctx := context.Background()
	r := ocitest.NewRegistry(t, ocimem.New())
	r.MustPushContent(ocitest.RegistryContent{
		"foo/bar": {
			Blobs: map[string]string{
				"b1":      "hello",
				"scratch": "{}",
			},
			Manifests: map[string]ociregistry.Manifest{
				"m1": {
					MediaType: ocispec.MediaTypeImageManifest,
					Config: ociregistry.Descriptor{
						Digest: "scratch",
					},
					Layers: []ociregistry.Descriptor{{
						Digest: "b1",
					}},
				},
			},
			Tags: map[string]string{
				"t1": "m1",
				"t2": "m1",
			},
		},
		"fooey": {
			Blobs: map[string]string{
				"scratch": "{}",
			},
			Manifests: map[string]ociregistry.Manifest{
				"m1": {
					MediaType: ocispec.MediaTypeImageManifest,
					Config: ociregistry.Descriptor{
						Digest: "scratch",
					},
				},
			},
			Tags: map[string]string{
				"t1": "m1",
			},
		},
		"other/blah": {
			Blobs: map[string]string{
				"scratch": "{}",
			},
			Manifests: map[string]ociregistry.Manifest{
				"m1": {
					MediaType: ocispec.MediaTypeImageManifest,
					Config: ociregistry.Descriptor{
						Digest: "scratch",
					},
				},
			},
			Tags: map[string]string{
				"t1": "m1",
			},
		},
	})
	r1 := ocitest.NewRegistry(t, Sub(r.R, "foo"))
	desc, err := r1.R.ResolveTag(ctx, "bar", "t1")
	qt.Assert(t, qt.IsNil(err))

	m := getManifest(t, r1.R, "bar", desc.Digest)
	b1Content := getBlob(t, r1.R, "bar", m.Layers[0].Digest)
	qt.Assert(t, qt.Equals(string(b1Content), "hello"))

	repos, err := ociregistry.All(r1.R.Repositories(ctx))
	qt.Assert(t, qt.IsNil(err))
	sort.Strings(repos)
	qt.Assert(t, qt.DeepEquals(repos, []string{"bar"}))
}

func TestSubMaintainsAuthScope(t *testing.T) {
	var gotScope ociauth.Scope
	r := Sub(contextChecker{
		check: func(ctx context.Context) {
			gotScope = ociauth.ScopeFromContext(ctx)
		},
	}, "foo/bar")
	scope := ociauth.ParseScope("other registry:catalog:* repository:a/b:pull,push repository:foo:delete,push")
	ctx := ociauth.ContextWithScope(context.Background(), scope)

	// As the implementation is so uniform (and easily inspected in the source,
	// we use the GetBlob entry point as a proxy for testing all the entry points.
	// TODO it would be nice to have a reusable way (in ocitest, probably) of testing general properties
	// across all ociregistry.Interface methods.
	_, _ = r.GetBlob(ctx, "some/repo", "sha256:fffff")
	qt.Assert(t, qt.DeepEquals(gotScope, ociauth.ParseScope(
		"other registry:catalog:* repository:foo/bar/a/b:pull,push repository:foo/bar/foo:delete,push",
	)))
}

type contextChecker struct {
	ociregistry.Interface
	check func(context.Context)
}

func (r contextChecker) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	r.check(ctx)
	return nil, fmt.Errorf("nope")
}

func getManifest(t *testing.T, r ociregistry.Interface, repo string, dg digest.Digest) ociregistry.Manifest {
	rd, err := r.GetManifest(context.Background(), repo, dg)
	qt.Assert(t, qt.IsNil(err))
	defer rd.Close()
	var m ociregistry.Manifest
	data, err := io.ReadAll(rd)
	qt.Assert(t, qt.IsNil(err))
	err = json.Unmarshal(data, &m)
	qt.Assert(t, qt.IsNil(err))
	return m
}

func getBlob(t *testing.T, r ociregistry.Interface, repo string, dg digest.Digest) []byte {
	rd, err := r.GetBlob(context.Background(), repo, dg)
	qt.Assert(t, qt.IsNil(err))
	defer rd.Close()
	data, err := io.ReadAll(rd)
	qt.Assert(t, qt.IsNil(err))
	return data
}
