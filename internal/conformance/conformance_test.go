package conformance

// Import the package so we have it available to run its tests.
import _ "github.com/opencontainers/distribution-spec/conformance"

//
//func TestConformance(t *testing.T) {
//	r := ociserver.New(ocimem.New(), nil)
//	srv := httptest.NewServer(r)
//	defer srv.Close()
//	cmd := exec.NewCommand("go", "test", "github.com/opencontainers/distribution-spec/conformance")
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//
//}
