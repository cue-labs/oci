package conformance

import (
	"bufio"
	"log"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/rogpeppe/ociregistry/ociclient"
	"github.com/rogpeppe/ociregistry/ocimem"
	"github.com/rogpeppe/ociregistry/ociserver"
)

func init() {
	log.SetFlags(log.Lmicroseconds)
}

func TestMem(t *testing.T) {
	srv := httptest.NewServer(ociserver.New(ocimem.New(), nil))
	defer srv.Close()
	testConformance(t, srv.URL)
}

func TestClientAsProxy(t *testing.T) {
	direct := httptest.NewServer(ociserver.New(ocimem.New(), &ociserver.Options{
		DebugID: "direct",
	}))
	defer direct.Close()
	proxy := httptest.NewServer(ociserver.New(ociclient.New(direct.URL), &ociserver.Options{
		DebugID: "proxy",
	}))
	defer proxy.Close()
	testConformance(t, proxy.URL)
}

func testConformance(t *testing.T, srvURL string) {
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

	args := []string{"test"}
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
