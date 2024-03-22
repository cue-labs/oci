//go:build go1.23

package ociregistry

import (
	"errors"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestSliceIter(t *testing.T) {
	slice := []int{3, 1, 4}
	var got []int
	for x, err := range SliceIter(slice) {
		qt.Assert(t, qt.IsNil(err))
		got = append(got, x)
	}
	qt.Assert(t, qt.DeepEquals(got, slice))
}

func TestErrorIter(t *testing.T) {
	err := errors.New("foo")
	i := 0
	for s, gotErr := range ErrorIter[string](err) {
		qt.Assert(t, qt.Equals(i, 0))
		qt.Assert(t, qt.Equals(s, ""))
		qt.Assert(t, qt.Equals(err, gotErr))
		i++
	}
	qt.Assert(t, qt.Equals(i, 1))
}
