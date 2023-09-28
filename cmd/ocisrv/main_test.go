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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"github.com/opencontainers/go-digest"
	"github.com/rogpeppe/go-internal/testscript"
	"github.com/rogpeppe/retry"
)

func init() {
	writeNetAddr = writeNetAddrForTest

	// Process the interrupt signal sent by testscript
	// to ensure a clean exit.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)
	go func() {
		<-sigc
		os.Exit(0)
	}()
}

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"ocisrv": func() int {
			main()
			return 0
		},
	}))
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			env.Setenv("ADDR_FILE", filepath.Join(env.WorkDir, "listen-addr"))
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"pushblob": cmdPushBlob,
		},
	})
}

func cmdPushBlob(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 3 {
		ts.Fatalf("usage: pushblob $repo $blobfile $digest")
	}
	repo, blobFile, dg := args[0], args[1], args[2]
	data, err := os.ReadFile(ts.MkAbs(blobFile))
	ts.Check(err)
	r, err := connect(ts)
	ts.Check(err)
	if !neg && digest.FromBytes(data) != digest.Digest(dg) {
		ts.Fatalf("blob digest mismatch")
	}

	_, err = r.PushBlob(context.Background(), repo, ociregistry.Descriptor{
		Size:   int64(len(data)),
		Digest: digest.Digest(dg),
	}, bytes.NewReader(data))
	if neg {
		if err == nil {
			ts.Fatalf("unexpected success")
		}
		// TODO it would be nice if we could write the error to stderr
		// so it could be checked by testscript commands.
		ts.Logf("error: %v", err)
		return
	}
	ts.Check(err)
}

func cmdGetBlob(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 2 {
		ts.Fatalf("usage: getblob $repo $digest")
	}
	repo, dg := args[0], args[1]
	r, err := connect(ts)
	ts.Check(err)
	rd, err := r.GetBlob(context.Background(), repo, digest.Digest(dg))
	if neg {
		if err == nil {
			ts.Fatalf("unexpected success")
		}
		// TODO it would be nice if we could write the error to stderr
		// so it could be checked by testscript commands.
		ts.Logf("error: %v", err)
		return
	}
	data, err := io.ReadAll(rd)
	ts.Check(err)
	_, err = ts.Stdout().Write(data)
	ts.Check(err)
}

var waitStrategy = retry.Strategy{
	Delay:       time.Millisecond,
	MaxDelay:    20 * time.Millisecond,
	MaxDuration: 500 * time.Millisecond,
}

func connect(ts *testscript.TestScript) (ociregistry.Interface, error) {
	addrFile := ts.Getenv("ADDR_FILE")
	if addrFile == "" {
		return nil, fmt.Errorf("$ADDR_FILE not set")
	}
	var addr string
	for it := waitStrategy.Start(); ; {
		data, err := os.ReadFile(addrFile)
		if err == nil {
			addr = string(data)
			break
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot read file: %v", err)
		}
		if !it.Next(nil) {
			return nil, fmt.Errorf("timed out waiting for server")
		}
	}
	resp, err := http.Get("http://" + addr + "/v2/")
	if err != nil {
		return nil, fmt.Errorf("cannot ping server: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected ping status (%v)", resp.Status)
	}
	return ociclient.New(addr, &ociclient.Options{
		Insecure: true,
	})
}

func writeNetAddrForTest(l net.Listener) {
	f := os.Getenv("ADDR_FILE")
	if f == "" {
		return
	}
	tmpf := f + ".tmp"
	if err := os.WriteFile(tmpf, []byte(l.Addr().String()), 0o666); err != nil {
		panic(err)
	}
	if err := os.Rename(tmpf, f); err != nil {
		panic(err)
	}
}
