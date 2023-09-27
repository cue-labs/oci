package ocifilter

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/internal/ocitest"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
)

func TestTrimPrefix(t *testing.T) {
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
