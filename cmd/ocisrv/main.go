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
	_ "embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"

	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"github.com/cue-exp/cueconfig"
	"github.com/go-json-experiment/json"
)

var (
	//go:embed schema.cue
	configSchema []byte

	//go:embed defaults.cue
	configDefaults []byte
)

type config struct {
	Registry   registry `json:"registry"`
	ListenAddr string   `json:"listenAddr"`
}

func main() {
	if err := main1(); err != nil {
		fmt.Fprintf(os.Stderr, "ociregistry: %v\n", err)
		os.Exit(1)
	}
}

var writeNetAddr func(l net.Listener)

func main1() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ocisrv $configfile.cue\n")
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	configFile := flag.Arg(0)

	// Don't decode into Go struct yet because we want to use
	// json v2 for that so we can decode into the registry interface
	// type.
	var cfgRaw json.RawValue
	if err := cueconfig.Load(configFile, configSchema, configDefaults, nil, &cfgRaw); err != nil {
		return err
	}
	cfg, err := unmarshalConfig(cfgRaw)
	if err != nil {
		return fmt.Errorf("cannot decode config: %v", err)
	}
	r, err := cfg.Registry.new()
	if err != nil {
		return fmt.Errorf("cannot construct registry: %v", err)
	}
	l, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("cannot listen on %q: %v", cfg.ListenAddr, err)
	}
	if writeNetAddr != nil {
		writeNetAddr(l)
	}
	fmt.Printf("listening on %v\n", l.Addr())
	err = http.Serve(l, ociserver.New(r, nil))
	return fmt.Errorf("http server error: %v", err)
}

func unmarshalConfig(cfgRaw []byte) (*config, error) {
	opts := &json.UnmarshalOptions{
		Unmarshalers: json.UnmarshalFuncV2(unmarshalRegistry),
	}
	var cfg config
	if err := opts.Unmarshal(json.DecodeOptions{}, cfgRaw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func unmarshalRegistry(opts json.UnmarshalOptions, dec *json.Decoder, rp *registry) error {
	var data json.RawValue
	if err := opts.UnmarshalNext(dec, &data); err != nil {
		return err
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := opts.Unmarshal(json.DecodeOptions{}, data, &kind); err != nil {
		return err
	}
	t := kindToRegistryType[kind.Kind]
	if t == nil {
		return fmt.Errorf("no registry type found for kind %q", kind.Kind)
	}
	r := reflect.New(t)
	if err := opts.Unmarshal(json.DecodeOptions{}, data, r.Interface()); err != nil {
		return err
	}
	*rp = r.Elem().Interface().(registry)
	return nil
}
