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

package conformance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"testing"

	"github.com/go-quicktest/qt"

	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocidebug"
	"cuelabs.dev/go/oci/ociregistry/ocifilter"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelabs.dev/go/oci/ociregistry/ociunify"
)

func init() {
	log.SetFlags(log.Lmicroseconds)
}

func TestMem(t *testing.T) {
	runTests(t, func(t *testing.T) string {
		srv := httptest.NewServer(ociserver.New(ocidebug.New(ocimem.New(), t.Logf), nil))
		t.Cleanup(srv.Close)
		return srv.URL
	})
}

func TestClientAsProxy(t *testing.T) {
	runTests(t, func(t *testing.T) string {
		direct := httptest.NewServer(ociserver.New(ocimem.New(), &ociserver.Options{
			DebugID: "direct",
		}))
		t.Cleanup(direct.Close)
		proxy := httptest.NewServer(ociserver.New(mustNewOCIClient(direct.URL, nil), &ociserver.Options{
			DebugID: "proxy",
		}))
		t.Cleanup(proxy.Close)
		return proxy.URL
	})
}

func TestSelectAll(t *testing.T) {
	// Test that the Select filter works OK as a no-op when
	// all repositories are allowed.
	runTests(t, func(t *testing.T) string {
		r := ocifilter.Select(ocimem.New(), func(string) bool {
			return true
		})
		srv := httptest.NewServer(ociserver.New(r, nil))
		t.Cleanup(srv.Close)
		return srv.URL
	})
}

func TestUnifyingProxySequential(t *testing.T) {
	testUnifyingProxy(t, &ociunify.Options{
		ReadPolicy: ociunify.ReadSequential,
	})
}

func TestUnifyingProxyConcurrent(t *testing.T) {
	testUnifyingProxy(t, &ociunify.Options{
		ReadPolicy: ociunify.ReadConcurrent,
	})
}

func testUnifyingProxy(t *testing.T, opts *ociunify.Options) {
	debugWrap := func(what string, r ociregistry.Interface) ociregistry.Interface {
		return ocidebug.New(r, func(f string, a ...any) {
			t.Logf("%s: %s", what, fmt.Sprintf(f, a...))
		})
	}
	runTests(t, func(t *testing.T) string {
		direct0 := httptest.NewServer(ociserver.New(debugWrap("direct0mem", ocimem.New()), &ociserver.Options{
			DebugID: "direct0srv",
		}))
		t.Cleanup(direct0.Close)
		direct1 := httptest.NewServer(ociserver.New(debugWrap("direct1mem", ocimem.New()), &ociserver.Options{
			DebugID: "direct1srv",
		}))
		t.Cleanup(direct1.Close)
		proxy := httptest.NewServer(ociserver.New(debugWrap("proxy", ociunify.New(
			debugWrap("proxy0", mustNewOCIClient(direct0.URL, &ociclient.Options{
				DebugID: "client0",
			})),
			debugWrap("proxy1", mustNewOCIClient(direct1.URL, &ociclient.Options{
				DebugID: "client1",
			})),
			opts,
		)), &ociserver.Options{
			DebugID: "proxysrv",
		}))
		t.Cleanup(proxy.Close)
		return proxy.URL
	})
}

func runTests(t *testing.T, startSrv func(t *testing.T) string) {
	t.Run("distribution", func(t *testing.T) {
		testDistribution(t, startSrv)
	})
	t.Run("extra", func(t *testing.T) {
		testExtra(t, startSrv)
	})
	t.Run("extraWithLocalClient", func(t *testing.T) {
		testExtraWithLocalClient(t, startSrv)
	})
}

var extraTests = []struct {
	testName string
	run      func(t *testing.T, client *remote.Registry)
}{{
	testName: "catalog",
	run:      testCatalog,
}, {
	testName: "referrers",
	run:      testReferrers,
}}

// testExtra runs a bunch of extra tests of functionality that isn't
// covered by the distribution's conformance tests.
//
// We use the oras-go client to keep us honest.
func testExtra(t *testing.T, startSrv func(*testing.T) string) {
	for _, test := range extraTests {
		t.Run(test.testName, func(t *testing.T) {
			srvURL := startSrv(t)
			u, err := url.Parse(srvURL)
			qt.Assert(t, qt.IsNil(err))
			client, err := remote.NewRegistry(u.Host)
			qt.Assert(t, qt.IsNil(err))
			client.PlainHTTP = true
			test.run(t, client)
		})
	}
}

func testCatalog(t *testing.T, client *remote.Registry) {
	repos := []string{
		"foo/bar",
		"zaphod",
		"something123/longer/xx",
	}
	ctx := context.Background()
	for _, repoName := range repos {
		repo, err := client.Repository(ctx, repoName)
		qt.Assert(t, qt.IsNil(err))
		push(t, repo.Blobs(), "", []byte("hello "+repoName))
	}

	var gotRepos []string
	err := client.Repositories(ctx, "", func(repos []string) error {
		gotRepos = append(gotRepos, repos...)
		return nil
	})
	qt.Assert(t, qt.IsNil(err))
	sort.Strings(repos)
	qt.Assert(t, qt.DeepEquals(gotRepos, repos))
}

func testReferrers(t *testing.T, client *remote.Registry) {
	ctx := context.Background()
	repo, err := client.Repository(ctx, "some/repo")
	qt.Assert(t, qt.IsNil(err))
	configDesc := push(t, repo.Blobs(), "application/json", []byte("{}"))
	layer0Desc := push(t, repo.Blobs(), "", []byte("some content"))
	manifestDesc := pushJSON(t, repo.Manifests(), ocispec.MediaTypeImageManifest, ociregistry.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    withMediaType(configDesc, "artifact1"),
		Layers:    []ociregistry.Descriptor{layer0Desc},
	})
	referrer0 := pushJSON(t, repo.Manifests(), ocispec.MediaTypeImageManifest, ociregistry.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    withMediaType(configDesc, "referrer0"),
		Subject:   &manifestDesc,
	})
	referrer1 := pushJSON(t, repo.Manifests(), ocispec.MediaTypeImageManifest, ociregistry.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    withMediaType(configDesc, "referrer1"),
		Subject:   &manifestDesc,
	})
	referrer2 := pushJSON(t, repo.Manifests(), ocispec.MediaTypeImageManifest, ociregistry.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    withMediaType(configDesc, "referrer2"),
		Subject:   &referrer1,
	})
	var gotReferrers []ocispec.Descriptor
	err = repo.Referrers(ctx, manifestDesc, "", func(referrers []ocispec.Descriptor) error {
		gotReferrers = append(gotReferrers, referrers...)
		return nil
	})
	qt.Assert(t, qt.IsNil(err))
	wantReferrers := []ociregistry.Descriptor{referrer0, referrer1}
	sortDescriptors(gotReferrers)
	sortDescriptors(wantReferrers)
	qt.Assert(t, qt.DeepEquals(gotReferrers, wantReferrers))

	gotReferrers = nil
	err = repo.Referrers(ctx, referrer1, "", func(referrers []ocispec.Descriptor) error {
		gotReferrers = append(gotReferrers, referrers...)
		return nil
	})
	qt.Assert(t, qt.IsNil(err))
	wantReferrers = []ociregistry.Descriptor{referrer2}
	sortDescriptors(gotReferrers)
	sortDescriptors(wantReferrers)
	qt.Assert(t, qt.DeepEquals(gotReferrers, wantReferrers))

}

var extraWithLocalClientTests = []struct {
	testName string
	run      func(t *testing.T, reg ociregistry.Interface)
}{{
	testName: "rangeInBounds",
	run:      testRangeInBounds,
}, {
	testName: "rangeToEnd",
	run:      testRangeToEnd,
}, {
	testName: "rangeBeyondEnd",
	run:      testRangeBeyondEnd,
}}

// testExtraWithLocalClient defines extra tests that can't use to oras-go client
// because they exercise functionality that it doesn't support.
func testExtraWithLocalClient(t *testing.T, startSrv func(*testing.T) string) {
	for _, test := range extraWithLocalClientTests {
		t.Run(test.testName, func(t *testing.T) {
			srvURL := startSrv(t)
			client := mustNewOCIClient(srvURL, nil)
			test.run(t, client)
		})
	}
}

func mustNewOCIClient(srvURL string, opts *ociclient.Options) ociregistry.Interface {
	if opts == nil {
		opts = new(ociclient.Options)
	}
	u, err := url.Parse(srvURL)
	if err != nil {
		panic(err)
	}
	client, err := ociclient.New(u.Host, &ociclient.Options{
		Insecure: u.Scheme == "http",
	})
	if err != nil {
		panic(err)
	}
	return client
}

func testRangeInBounds(t *testing.T, reg ociregistry.Interface) {
	ctx := context.Background()
	data := []byte("hello world")
	desc, err := reg.PushBlob(ctx, "foo/bar", ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Size:      int64(len(data)),
		Digest:    digest.FromBytes(data),
	}, bytes.NewReader(data))
	qt.Assert(t, qt.IsNil(err))
	r, err := reg.GetBlobRange(ctx, "foo/bar", desc.Digest, 6, 8)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.DeepEquals(r.Descriptor(), desc))
	gotData, err := io.ReadAll(r)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(gotData), "wo"))
}

func testRangeToEnd(t *testing.T, reg ociregistry.Interface) {
	ctx := context.Background()
	data := []byte("hello world")
	desc, err := reg.PushBlob(ctx, "foo/bar", ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Size:      int64(len(data)),
		Digest:    digest.FromBytes(data),
	}, bytes.NewReader(data))
	qt.Assert(t, qt.IsNil(err))
	r, err := reg.GetBlobRange(ctx, "foo/bar", desc.Digest, 1, -1)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.DeepEquals(r.Descriptor(), desc))
	gotData, err := io.ReadAll(r)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(gotData), "ello world"))
}

func testRangeBeyondEnd(t *testing.T, reg ociregistry.Interface) {
	ctx := context.Background()
	data := []byte("hello world")
	desc, err := reg.PushBlob(ctx, "foo/bar", ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Size:      int64(len(data)),
		Digest:    digest.FromBytes(data),
	}, bytes.NewReader(data))
	qt.Assert(t, qt.IsNil(err))
	r, err := reg.GetBlobRange(ctx, "foo/bar", desc.Digest, 1, 5000)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.DeepEquals(r.Descriptor(), desc))
	gotData, err := io.ReadAll(r)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(string(gotData), "ello world"))
}

func sortDescriptors(ds []ociregistry.Descriptor) {
	sort.Slice(ds, func(i, j int) bool {
		return ds[i].Digest < ds[j].Digest
	})
}

func withMediaType(desc ociregistry.Descriptor, mt string) ociregistry.Descriptor {
	desc.MediaType = mt
	return desc
}

func pushJSON(t *testing.T, dst content.Pusher, mediaType string, content any) ociregistry.Descriptor {
	data, err := json.Marshal(content)
	qt.Assert(t, qt.IsNil(err))
	return push(t, dst, mediaType, data)
}

func push(t *testing.T, dst content.Pusher, mediaType string, content []byte) ociregistry.Descriptor {
	desc := newDescriptor(mediaType, content)
	err := dst.Push(context.Background(), desc, bytes.NewReader(content))
	qt.Assert(t, qt.IsNil(err))
	return desc
}

func newDescriptor(mediaType string, content []byte) ociregistry.Descriptor {
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return ociregistry.Descriptor{
		MediaType: mediaType,
		Size:      int64(len(content)),
		Digest:    digest.FromBytes(content),
	}
}

// testDistribution exercises the distribution-spec conformance tests.
func testDistribution(t *testing.T, startSrv func(*testing.T) string) {
	srvURL := startSrv(t)
	// The conformance tests aren't available to run directly, so we
	// run `go test` on them.
	t.Setenv("OCI_ROOT_URL", srvURL)
	t.Setenv("OCI_NAMESPACE", "myorg/myrepo")
	t.Setenv("OCI_CROSSMOUNT_NAMESPACE", "myorg/other")
	t.Setenv("OCI_USERNAME", "myuser")
	t.Setenv("OCI_PASSWORD", "mypass'")
	t.Setenv("OCI_TEST_PULL", "1")
	t.Setenv("OCI_TEST_PUSH", "1")
	t.Setenv("OCI_TEST_CONTENT_DISCOVERY", "1")
	t.Setenv("OCI_TEST_CONTENT_MANAGEMENT", "1")
	t.Setenv("OCI_HIDE_SKIPPED_WORKFLOWS", "0")
	t.Setenv("OCI_DEBUG", "1")
	t.Setenv("OCI_DELETE_MANIFEST_BEFORE_BLOBS", "0")
	t.Setenv("ACK_GINKGO_DEPRECATIONS", "1.16.5")
	if os.Getenv("OCI_REPORT_DIR") == "" {
		t.Setenv("OCI_REPORT_DIR", "none")
	}

	// Note: if we don't have `-count=1` here, re-running the outer
	// test suite with `-count=1` won't actually force a re-run of the inner tests.
	args := []string{"test", "-count=1"}
	if testing.Verbose() {
		args = append(args, "-v")
	}
	args = append(args, "github.com/opencontainers/distribution-spec/conformance")

	cmd := exec.Command("go", args...)
	r, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for scan := bufio.NewScanner(r); scan.Scan(); {
		t.Log(scan.Text())
	}
	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			t.Fail()
		} else {
			t.Fatalf("unexpected error running command: %v", err)
		}
	}
}
