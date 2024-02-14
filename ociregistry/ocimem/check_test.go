package ocimem

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocitest"
	"github.com/go-quicktest/qt"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

var pushManifestTests = []struct {
	testName     string
	preload      ocitest.RepoContent
	config       Config
	tag          string
	mediaType    string
	manifestData func(content ocitest.PushedRepoContent) []byte
	wantError    string
}{{
	testName:  "NonExistentConfigReference",
	mediaType: ocispec.MediaTypeImageManifest,
	manifestData: func(ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config: ociregistry.Descriptor{
				MediaType: "application/something",
				Size:      1,
				Digest:    digest.FromString("a"),
			},
		})
	},
	wantError: `invalid manifest: blob for config not found`,
}, {
	testName: "NonExistentLayerReference",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
	},
	mediaType: ocispec.MediaTypeImageManifest,
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    content.Blobs["a"],
			Layers: []ociregistry.Descriptor{{
				MediaType: "application/something",
				Size:      1,
				Digest:    digest.FromString("b"),
			}},
		})
	},
	wantError: `invalid manifest: blob for layers\[0\] not found`,
}, {
	testName: "NonExistentSubjectReference",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
	},
	mediaType: ocispec.MediaTypeImageManifest,
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    content.Blobs["a"],
			Subject: &ociregistry.Descriptor{
				MediaType: "application/something",
				Size:      1,
				Digest:    digest.FromString("b"),
			},
		})
	},
	// Non-existent subject references are explicitly allowed.
}, {
	testName:  "NonExistentImageIndexManifestReference",
	mediaType: ocispec.MediaTypeImageIndex,
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ocispec.Index{
			MediaType: ocispec.MediaTypeImageIndex,
			Manifests: []ociregistry.Descriptor{{
				MediaType: ocispec.MediaTypeImageManifest,
				Size:      1,
				Digest:    digest.FromString("a"),
			}},
		})
	},
	wantError: `invalid manifest: manifest for manifests\[0\] not found`,
}, {
	testName:  "NonExistentImageIndexSubjectReference",
	mediaType: ocispec.MediaTypeImageIndex,
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ocispec.Index{
			MediaType: ocispec.MediaTypeImageIndex,
			Subject: &ociregistry.Descriptor{
				MediaType: "application/something",
				Size:      1,
				Digest:    digest.FromString("b"),
			},
		})
	},
	// Non-existent subject references are explicitly allowed.
}, {
	testName: "CannotOverwriteTagWhenImmutabilityEnabled",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
				Layers: []ociregistry.Descriptor{{
					Digest: "a",
				}},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	config: Config{
		ImmutableTags: true,
	},
	mediaType: ocispec.MediaTypeImageManifest,
	tag:       "sometag",
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    content.Blobs["a"],
			Layers:    []ociregistry.Descriptor{content.Blobs["a"]},
			Annotations: map[string]string{
				"different": "thing",
			},
		})
	},
	wantError: `requested access to the resource is denied: cannot overwrite tag`,
}, {
	testName: "CanRewriteTagWithIdenticalContentsWhenImmutabilityEnabled",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
				Layers: []ociregistry.Descriptor{{
					Digest: "a",
				}},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	config: Config{
		ImmutableTags: true,
	},
	mediaType: ocispec.MediaTypeImageManifest,
	tag:       "sometag",
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return content.ManifestData["m"]
	},
}, {
	testName: "CannotRewriteTagWithIdenticalContentsButDifferentMediaTypeWhenImmutabilityEnabled",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
				Layers: []ociregistry.Descriptor{{
					Digest: "a",
				}},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	config: Config{
		ImmutableTags: true,
	},
	mediaType: "application/vnd.docker.container.image.v1+json",
	tag:       "sometag",
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return content.ManifestData["m"]
	},
	wantError: `requested access to the resource is denied: mismatched media type`,
}, {
	testName: "CanOverwriteTagWhenImmutabilityNotEnabled",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
				Layers: []ociregistry.Descriptor{{
					Digest: "a",
				}},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	mediaType: ocispec.MediaTypeImageManifest,
	tag:       "sometag",
	manifestData: func(content ocitest.PushedRepoContent) []byte {
		return mustJSONMarshal(ociregistry.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    content.Blobs["a"],
			Layers:    []ociregistry.Descriptor{content.Blobs["a"]},
			Annotations: map[string]string{
				"different": "thing",
			},
		})
	},
}}

func TestPushManifest(t *testing.T) {
	for _, test := range pushManifestTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := context.Background()
			r := ocitest.NewRegistry(t, NewWithConfig(&test.config))
			content := r.MustPushContent(ocitest.RegistryContent{
				"test": test.preload,
			})["test"]
			data := test.manifestData(content)
			_, err := r.R.PushManifest(ctx, "test", test.tag, data, test.mediaType)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
			} else {
				qt.Assert(t, qt.IsNil(err))
			}
		})
	}
}

var deleteBlobTests = []struct {
	testName  string
	config    Config
	preload   ocitest.RepoContent
	getDigest func(content ocitest.PushedRepoContent) ociregistry.Digest
	wantError string
}{{
	testName: "NonExistentRepo",
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return digest.FromString("blshdfsvg")
	},
	wantError: "repository name not known to registry",
}, {
	testName: "NonExistentBlob",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return digest.FromString("blshdfsvg")
	},
	wantError: "blob unknown to registry",
}, {
	testName: "TaggedBlobWithImmutableTags",
	config: Config{
		ImmutableTags: true,
	},
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
				Layers: []ociregistry.Descriptor{{
					Digest: "a",
				}},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return content.Blobs["a"].Digest
	},
	wantError: "requested access to the resource is denied: deletion of tagged blob not permitted",
}, {
	testName: "IndirectlyTaggedBlobWithImmutableTags",
	config: Config{
		ImmutableTags: true,
	},
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m0": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
			},
			"m1": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "b",
				},
				Subject: &ociregistry.Descriptor{
					Digest: "m0",
				},
			},
		},
		Tags: map[string]string{
			"sometag": "m1",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return content.Blobs["a"].Digest
	},
	wantError: "requested access to the resource is denied: deletion of tagged blob not permitted",
}}

func TestDeleteBlob(t *testing.T) {
	for _, test := range deleteBlobTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := context.Background()
			r := ocitest.NewRegistry(t, NewWithConfig(&test.config))
			content := r.MustPushContent(ocitest.RegistryContent{
				"test": test.preload,
			})["test"]
			digest := test.getDigest(content)
			err := r.R.DeleteBlob(ctx, "test", digest)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
			} else {
				qt.Assert(t, qt.IsNil(err))
			}
			// Regardless of the result, the blob shouldn't be there afterwards
			// unless the operation was denied.
			if !errors.Is(err, ociregistry.ErrDenied) {
				_, err := r.R.ResolveBlob(ctx, "test", digest)
				qt.Assert(t, qt.Not(qt.IsNil(err)))
			}
		})
	}
}

var deleteManifestTests = []struct {
	testName  string
	config    Config
	preload   ocitest.RepoContent
	getDigest func(content ocitest.PushedRepoContent) ociregistry.Digest
	wantError string
}{{
	testName: "NonExistentRepo",
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return digest.FromString("blshdfsvg")
	},
	wantError: "repository name not known to registry",
}, {
	testName: "NonExistentManifest",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return digest.FromString("blshdfsvg")
	},
	wantError: "manifest unknown to registry",
}, {
	testName: "TaggedManifestWithImmutableTags",
	config: Config{
		ImmutableTags: true,
	},
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return content.Manifests["m"].Digest
	},
	wantError: "requested access to the resource is denied: deletion of tagged manifest not permitted",
}, {
	testName: "IndirectlyTaggedManifestWithImmutableTags",
	config: Config{
		ImmutableTags: true,
	},
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m0": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
			},
			"m1": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "b",
				},
				Subject: &ociregistry.Descriptor{
					Digest: "m0",
				},
			},
		},
		Tags: map[string]string{
			"sometag": "m1",
		},
	},
	getDigest: func(content ocitest.PushedRepoContent) ociregistry.Digest {
		return content.Manifests["m0"].Digest
	},
	wantError: "requested access to the resource is denied: deletion of tagged manifest not permitted",
}}

func TestDeleteManifest(t *testing.T) {
	for _, test := range deleteManifestTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := context.Background()
			r := ocitest.NewRegistry(t, NewWithConfig(&test.config))
			content := r.MustPushContent(ocitest.RegistryContent{
				"test": test.preload,
			})["test"]
			digest := test.getDigest(content)
			err := r.R.DeleteManifest(ctx, "test", digest)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
			} else {
				qt.Assert(t, qt.IsNil(err))
			}
			// Regardless of the result, the manifest shouldn't be there afterwards
			// unless the operation was denied.
			if !errors.Is(err, ociregistry.ErrDenied) {
				_, err := r.R.ResolveManifest(ctx, "test", digest)
				qt.Assert(t, qt.Not(qt.IsNil(err)))
			}
		})
	}
}

var deleteTagTests = []struct {
	testName  string
	config    Config
	preload   ocitest.RepoContent
	tag       string
	wantError string
}{{
	testName:  "NonExistentRepo",
	tag:       "foo",
	wantError: "repository name not known to registry",
}, {
	testName: "NonExistentTag",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
	},
	tag:       "foo",
	wantError: "manifest unknown to registry: tag does not exist",
}, {
	testName: "WithImmutableTags",
	config: Config{
		ImmutableTags: true,
	},
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
			},
		},
		Tags: map[string]string{
			"sometag": "m",
		},
	},
	tag:       "sometag",
	wantError: "requested access to the resource is denied: tag deletion not permitted",
}, {
	testName: "Success",
	preload: ocitest.RepoContent{
		Blobs: map[string]string{
			"a": "{}",
			"b": "other",
		},
		Manifests: map[string]ociregistry.Manifest{
			"m0": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "a",
				},
			},
			"m1": {
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ociregistry.Descriptor{
					Digest: "b",
				},
				Subject: &ociregistry.Descriptor{
					Digest: "m0",
				},
			},
		},
		Tags: map[string]string{
			"sometag": "m1",
		},
	},
	tag: "sometag",
}}

func TestDeleteTag(t *testing.T) {
	for _, test := range deleteTagTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := context.Background()
			r := ocitest.NewRegistry(t, NewWithConfig(&test.config))
			content := r.MustPushContent(ocitest.RegistryContent{
				"test": test.preload,
			})["test"]
			err := r.R.DeleteTag(ctx, "test", test.tag)
			if test.wantError != "" {
				qt.Assert(t, qt.ErrorMatches(err, test.wantError))
			} else {
				qt.Assert(t, qt.IsNil(err))
			}
			// Regardless of the result, the tag shouldn't be there afterwards
			// unless the operation was denied.
			if !errors.Is(err, ociregistry.ErrDenied) {
				_, err := r.R.ResolveTag(ctx, "test", test.tag)
				qt.Assert(t, qt.Not(qt.IsNil(err)))
			}
			// The manifest should remain present even though the tag
			// itself has been deleted.
			if tagDesc, ok := content.Manifests[test.preload.Tags[test.tag]]; ok {
				_, err := r.R.ResolveManifest(ctx, "test", tagDesc.Digest)
				qt.Assert(t, qt.IsNil(err))
			}
		})
	}
}

func mustJSONMarshal(x any) []byte {
	data, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return data
}
