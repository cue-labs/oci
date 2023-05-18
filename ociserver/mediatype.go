package ociregistry

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	mediaTypeDockerManifestList    = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaTypeDockerForeignLayer    = "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip"
	mediaTypeDockerManifestSchema2 = "application/vnd.docker.distribution.manifest.v2+json"

	mediaTypeOCIImageIndex                  = ocispec.MediaTypeImageIndex
	mediaTypeOCIRestrictedLayer             = ocispec.MediaTypeImageLayerNonDistributableGzip
	mediaTypeOCIUncompressedRestrictedLayer = ocispec.MediaTypeImageLayerNonDistributable
	mediaTypeOCIManifestSchema1             = ocispec.MediaTypeImageManifest
	mediaTypeOCIConfigJSON                  = ocispec.MediaTypeImageConfig
	mediaTypeDockerConfigJSON               = "application/vnd.docker.container.image.v1+json"
)

func isIndex(mt string) bool {
	return mt == mediaTypeOCIImageIndex ||
		mt == mediaTypeDockerManifestList
}

func isDistributable(mt string) bool {
	return mt != mediaTypeDockerForeignLayer &&
		mt != mediaTypeOCIRestrictedLayer &&
		mt != mediaTypeOCIUncompressedRestrictedLayer
}

func isImage(mt string) bool {
	return mt == mediaTypeOCIManifestSchema1 ||
		mt == mediaTypeDockerManifestSchema2
}
