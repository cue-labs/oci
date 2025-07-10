package ocimem

import (
	"cmp"
	"encoding/json"
	"fmt"
	"iter"

	"cuelabs.dev/go/oci/ociregistry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type refKind int

const (
	kindSubjectManifest refKind = iota
	kindBlob
	kindManifest
)

type descInfo struct {
	name string
	kind refKind
	desc ociregistry.Descriptor
}

type manifestInfo struct {
	// descriptors iterates over all direct references inside the manifest
	descriptors descIter
	// subject holds the subject (referred-to manifest) of the manifest
	subject ociregistry.Digest
	// artifactType holds the artifact type of the manifest
	artifactType string
	// annotations holds any annotations from the manifest.
	annotations map[string]string
}

type descIter = iter.Seq[descInfo]

// TODO support other manifest types.
var manifestInfoByMediaType = map[string]func(data []byte) (manifestInfo, error){
	ocispec.MediaTypeImageManifest: manifestInfoForType(imageInfo),
	ocispec.MediaTypeImageIndex:    manifestInfoForType(indexInfo),
}

// getManifestInfo returns information on the manifest
// described by the given media type and data.
func getManifestInfo(mediaType string, data []byte) (manifestInfo, error) {
	getInfo := manifestInfoByMediaType[mediaType]
	if getInfo == nil {
		// TODO provide a configuration option to disallow unknown manifest types.
		//return nil, fmt.Errorf("media type %q: %w", mediaType, errUnknownManifestMediaTypeForIteration)
		return manifestInfo{
			descriptors: func(func(descInfo) bool) {},
		}, nil
	}
	return getInfo(data)
}

// repoTagIter returns an iterator that iterates through
// all the tags in the given repository.
func repoTagIter(r *repository) descIter {
	return func(yield func(descInfo) bool) {
		for tag, desc := range r.tags {
			if !yield(descInfo{
				name: tag,
				desc: desc,
				kind: kindManifest,
			}) {
				break
			}
		}
	}
}

func manifestInfoForType[T any](getInfo func(T) manifestInfo) func(data []byte) (manifestInfo, error) {
	return func(data []byte) (manifestInfo, error) {
		var x T
		if err := json.Unmarshal(data, &x); err != nil {
			return manifestInfo{}, fmt.Errorf("cannot unmarshal into %T: %v", &x, err)
		}
		return getInfo(x), nil
	}
}

func imageInfo(m ociregistry.Manifest) manifestInfo {
	var info manifestInfo
	info.descriptors = func(yield func(descInfo) bool) {
		for i, layer := range m.Layers {
			if !yield(descInfo{
				name: fmt.Sprintf("layers[%d]", i),
				desc: layer,
				kind: kindBlob,
			}) {
				return
			}
		}
		if !yield(descInfo{
			name: "config",
			desc: m.Config,
			kind: kindBlob,
		}) {
			return
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name: "subject",
				kind: kindSubjectManifest,
				desc: *m.Subject,
			}) {
				return
			}
		}
	}
	// From https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers
	//
	// The descriptors MUST include an artifactType field that is set to
	// the value of the artifactType in the image manifest or index, if
	// present. If the artifactType is empty or missing in the image
	// manifest, the value of artifactType MUST be set to the config
	// descriptor mediaType value. If the artifactType is empty or
	// missing in an index, the artifactType MUST be omitted. The
	// descriptors MUST include annotations from the image manifest or
	// index.
	info.artifactType = cmp.Or(m.ArtifactType, m.Config.MediaType)
	info.annotations = m.Annotations
	if m.Subject != nil {
		info.subject = m.Subject.Digest
	}
	return info
}

func indexInfo(m ocispec.Index) manifestInfo {
	var info manifestInfo
	info.descriptors = func(yield func(descInfo) bool) {
		for i, manifest := range m.Manifests {
			if !yield(descInfo{
				name: fmt.Sprintf("manifests[%d]", i),
				kind: kindManifest,
				desc: manifest,
			}) {
				return
			}
		}
		if m.Subject != nil {
			if !yield(descInfo{
				name: "subject",
				kind: kindSubjectManifest,
				desc: *m.Subject,
			}) {
				return
			}
		}
	}
	info.artifactType = m.ArtifactType // Note: no config descriptor to fall back to.
	info.annotations = m.Annotations
	if m.Subject != nil {
		info.subject = m.Subject.Digest
	}
	return info
}
