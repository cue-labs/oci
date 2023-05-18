// Copyright 2020 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hasher

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

func mustHash(s string, t *testing.T) Hash {
	h, _, err := SHA256(strings.NewReader(s))
	if err != nil {
		t.Fatalf("SHA256(%s) = %v", s, err)
	}
	t.Logf("Hashed: %q -> %q", s, h)
	return h
}

func TestVerificationFailure(t *testing.T) {
	want := "This is the input string."
	buf := bytes.NewBufferString(want)

	verified, err := ReadCloser(io.NopCloser(buf), int64(len(want)), mustHash("not the same", t))
	if err != nil {
		t.Fatal("ReadCloser() =", err)
	}
	if b, err := io.ReadAll(verified); err == nil {
		t.Errorf("ReadAll() = %q; want verification error", string(b))
	}
}

func TestVerification(t *testing.T) {
	want := "This is the input string."
	buf := bytes.NewBufferString(want)

	verified, err := ReadCloser(io.NopCloser(buf), int64(len(want)), mustHash(want, t))
	if err != nil {
		t.Fatal("ReadCloser() =", err)
	}
	if _, err := io.ReadAll(verified); err != nil {
		t.Error("ReadAll() =", err)
	}
}

func TestVerificationSizeUnknown(t *testing.T) {
	want := "This is the input string."
	buf := bytes.NewBufferString(want)

	verified, err := ReadCloser(io.NopCloser(buf), SizeUnknown, mustHash(want, t))
	if err != nil {
		t.Fatal("ReadCloser() =", err)
	}
	if _, err := io.ReadAll(verified); err != nil {
		t.Error("ReadAll() =", err)
	}
}

func TestBadHash(t *testing.T) {
	h := Hash{
		Algorithm: "fake256",
		Hex:       "whatever",
	}
	_, err := ReadCloser(io.NopCloser(strings.NewReader("hi")), 0, h)
	if err == nil {
		t.Errorf("ReadCloser() = %v, wanted err", err)
	}
}

func TestBadSize(t *testing.T) {
	want := "This is the input string."

	// having too much content or expecting too much content returns an error.
	for _, size := range []int64{3, 100} {
		t.Run(fmt.Sprintf("expecting size %d", size), func(t *testing.T) {
			buf := bytes.NewBufferString(want)
			rc, err := ReadCloser(io.NopCloser(buf), size, mustHash(want, t))
			if err != nil {
				t.Fatal("ReadCloser() =", err)
			}
			if b, err := io.ReadAll(rc); err == nil {
				t.Errorf("ReadAll() = %q; want verification error", string(b))
			}
		})
	}
}
