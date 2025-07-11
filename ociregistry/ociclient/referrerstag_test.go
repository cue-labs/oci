package ociclient

import (
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/go-quicktest/qt"
)

var referrersTagTests = []struct {
	digest ociregistry.Digest
	want   string
}{{
	// Test case from the distribution spec.
	digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	want:   "sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
}, {
	// Test case from the distribution spec.
	digest: "sha512:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	want:   "sha512-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
}, {
	// Test case from the distribution spec.
	digest: "test+algorithm+using+algorithm+separators+and+lots+of+characters+to+excercise+overall+truncation:alsoSome=InTheEncodedSectionToShowHyphenReplacementAndLotsAndLotsOfCharactersToExcerciseEncodedTruncation",
	want:   "test-algorithm-using-algorithm-s-alsoSome-InTheEncodedSectionToShowHyphenReplacementAndLotsAndLot",
}}

func TestReferrersTag(t *testing.T) {
	for _, test := range referrersTagTests {
		t.Run(string(test.digest), func(t *testing.T) {
			qt.Assert(t, qt.Equals(referrersTag(test.digest), test.want))
		})
	}
}
