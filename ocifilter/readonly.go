package ocifilter

import "github.com/rogpeppe/ociregistry"

func ReadOnly(r ociregistry.Interface) ociregistry.Interface {
	return struct {
		*ociregistry.Funcs
	}{}
}
