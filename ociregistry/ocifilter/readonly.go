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

import "cuelabs.dev/go/oci/ociregistry"

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
