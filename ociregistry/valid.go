package ociregistry

import (
	"regexp"

	"cuelabs.dev/go/oci/ociregistry/ociref"
)

var (
	tagPattern      = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
	repoNamePattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
)

// IsValidRepoName reports whether the given repository
// name is valid according to the specification.
//
// Deprecated: use [ociref.IsValidRepository].
func IsValidRepoName(repoName string) bool {
	return ociref.IsValidRepository(repoName)
}

// IsValidTag reports whether the digest d is valid
// according to the specification.
//
// Deprecated: use [ociref.IsValidTag].
func IsValidTag(tag string) bool {
	return ociref.IsValidTag(tag)
}

// IsValidDigest reports whether the digest d is well formed.
//
// Deprecated: use [ociref.IsValidDigest].
func IsValidDigest(d string) bool {
	return ociref.IsValidDigest(d)
}
