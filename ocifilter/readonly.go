package ocifilter

import "go.cuelabs.dev/ociregistry"

// ReadOnly returns a registry implementation that returns
// an "operation unsupported" error from all entry points that
// mutate the registry.
func ReadOnly(r ociregistry.Interface) ociregistry.Interface {
	// One level deeper so the Reader and Lister values take precedence,
	// following Go's shallower-method-wins rules.
	type deeper struct {
		*ociregistry.Funcs
	}
	return struct {
		ociregistry.Reader
		ociregistry.Lister
		deeper
	}{
		Reader: r,
		Lister: r,
	}
}
