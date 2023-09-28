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

package main

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocidebug"
	"cuelabs.dev/go/oci/ociregistry/ocifilter"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociunify"
)

var kindToRegistryType = make(map[string]reflect.Type)

func init() {
	for _, r := range []registry{
		clientRegistry{},
		selectRegistry{},
		readOnlyRegistry{},
		immutableRegistry{},
		unifyRegistry{},
		memRegistry{},
		debugRegistry{},
	} {
		t := reflect.TypeOf(r)
		name, ok := strings.CutSuffix(t.Name(), "Registry")
		if !ok {
			panic(fmt.Errorf("type %v has malformed name", name))
		}
		kindToRegistryType[name] = t
	}
}

type registry interface {
	new() (ociregistry.Interface, error)
}

type clientRegistry struct {
	Host     string `json:"host"`
	Insecure bool   `json:"insecure"`
	DebugID  string `json:"debugID,omitempty"`
}

func (r clientRegistry) new() (ociregistry.Interface, error) {
	return ociclient.New(r.Host, &ociclient.Options{
		DebugID:  r.DebugID,
		Insecure: r.Insecure,
	})
}

type selectRegistry struct {
	Registry registry       `json:"registry"`
	Include  *regexp.Regexp `json:"include,omitempty"`
	Exclude  *regexp.Regexp `json:"exclude,omitempty"`
}

func (r selectRegistry) new() (ociregistry.Interface, error) {
	r1, err := r.Registry.new()
	if err != nil {
		return nil, err
	}
	return ocifilter.Select(r1, func(repo string) bool {
		if !r.Include.MatchString(repo) {
			return false
		}
		if r.Exclude.MatchString(repo) {
			return false
		}
		return true
	}), nil
}

type readOnlyRegistry struct {
	Registry registry `json:"registry"`
}

func (r readOnlyRegistry) new() (ociregistry.Interface, error) {
	r1, err := r.Registry.new()
	if err != nil {
		return nil, err
	}
	return ocifilter.ReadOnly(r1), nil
}

type immutableRegistry struct {
	Registry registry `json:"registry"`
}

func (r immutableRegistry) new() (ociregistry.Interface, error) {
	r1, err := r.Registry.new()
	if err != nil {
		return nil, err
	}
	return ocifilter.Immutable(r1), nil
}

type unifyRegistry struct {
	Registries []registry `json:"registries"`
	// TODO options
}

func (r unifyRegistry) new() (ociregistry.Interface, error) {
	if len(r.Registries) != 2 {
		return nil, fmt.Errorf("can currently unify exactly two registries only")
	}
	r1 := make([]ociregistry.Interface, len(r.Registries))
	for i := range r.Registries {
		ri, err := r.Registries[i].new()
		if err != nil {
			return nil, err
		}
		r1[i] = ri
	}
	return ociunify.New(r1[0], r1[1], nil), nil
}

type memRegistry struct{}

func (r memRegistry) new() (ociregistry.Interface, error) {
	return ocimem.New(), nil
}

type debugRegistry struct {
	Registry registry `json:"registry"`
}

func (r debugRegistry) new() (ociregistry.Interface, error) {
	r1, err := r.Registry.new()
	if err != nil {
		return nil, err
	}
	return ocidebug.New(r1, nil), nil
}
