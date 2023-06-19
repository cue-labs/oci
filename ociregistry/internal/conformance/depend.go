//go:build donotincludeme

// Note: this file is here so we have the conformance package as part
// of our dependencies, so we're sure to be able to run `go test` on it.
// We can't import it from conformance_test.go because it has init-time
// functions that fail if the correct env vars aren't set.

package conformance

import _ "github.com/opencontainers/distribution-spec/conformance"
