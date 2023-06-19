package ociserver

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	mediaTypeOCIImageIndex                  = ocispec.MediaTypeImageIndex
	mediaTypeOCIRestrictedLayer             = ocispec.MediaTypeImageLayerNonDistributableGzip
	mediaTypeOCIUncompressedRestrictedLayer = ocispec.MediaTypeImageLayerNonDistributable
	mediaTypeOCIManifestSchema1             = ocispec.MediaTypeImageManifest
	mediaTypeOCIConfigJSON                  = ocispec.MediaTypeImageConfig
	mediaTypeDockerConfigJSON               = "application/vnd.docker.container.image.v1+json"
)
