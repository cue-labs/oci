package ociunify

import (
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/go-quicktest/qt"
)

var mergeIterTests = []struct {
	testName string
	it0, it1 ociregistry.Iter[int]
	want     []int
	wantErr  error
}{{
	testName: "IdenticalContents",
	it0:      ociregistry.SliceIter([]int{1, 2, 3}),
	it1:      ociregistry.SliceIter([]int{1, 2, 3}),
	want:     []int{1, 2, 3},
}, {
	testName: "DifferentContents",
	it0:      ociregistry.SliceIter([]int{0, 1, 2, 3}),
	it1:      ociregistry.SliceIter([]int{1, 2, 3, 5}),
	want:     []int{0, 1, 2, 3, 5},
}, {
	testName: "NoItems",
	it0:      ociregistry.SliceIter[int](nil),
	it1:      ociregistry.SliceIter[int](nil),
	want:     []int{},
}}

func TestMergeIter(t *testing.T) {
	for _, test := range mergeIterTests {
		t.Run(test.testName, func(t *testing.T) {
			it := mergeIter(test.it0, test.it1, cmpInt)
			xs, err := ociregistry.All(it)
			qt.Assert(t, qt.DeepEquals(xs, test.want))
			qt.Assert(t, qt.Equals(err, test.wantErr))
		})
	}
}

func cmpInt(i, j int) int {
	if i < j {
		return -1
	}
	if i > j {
		return 1
	}
	return 0
}
