package ocitest

import (
	"context"
	"strings"

	"cuelabs.dev/go/oci/ociregistry"
	"github.com/opencontainers/go-digest"
)

type MethodCall = func(ctx context.Context, r ociregistry.Interface) error

//go:generate go run golang.org/x/tools/cmd/stringer@v0.14.0 -type Method

type Method int

const (
	UnknownMethod Method = iota
	GetManifest
	GetBlob
	GetBlobRange
	GetTag
	ResolveBlob
	ResolveManifest
	ResolveTag
	PushBlob
	PushBlobChunked
	PushBlobChunkedResume
	MountBlob
	PushManifest
	DeleteBlob
	DeleteManifest
	DeleteTag
	Repositories
	Tags
	Referrers
)

// MethodCalls returns an iterator that produces an element
// for each ociregistry.Interface method holding the name
// of that method and a function that can call that method
// on a registry.
// Read operations always act on the repository foo/read;
// write operations act on foo/write.
// Other arguments are arbitrary.
//
// NOTE: this API is experimental and may change arbitrarily
// in future updates.
func MethodCalls() func(yield func(method Method, call MethodCall) bool) {
	return func(yield func(method Method, call MethodCall) bool) {
		stopped := false
		yield1 := func(method Method, call MethodCall) {
			if stopped {
				return
			}
			stopped = !yield(method, call)
		}

		yield1(GetBlob, func(ctx context.Context, r ociregistry.Interface) error {
			rd, err := r.GetBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			if rd != nil {
				rd.Close()
			}
			return err
		})
		yield1(GetBlobRange, func(ctx context.Context, r ociregistry.Interface) error {
			rd, err := r.GetBlobRange(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 100, 200)
			if rd != nil {
				rd.Close()
			}
			return err
		})
		yield1(GetManifest, func(ctx context.Context, r ociregistry.Interface) error {
			rd, err := r.GetManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			if rd != nil {
				rd.Close()
			}
			return err
		})
		yield1(GetTag, func(ctx context.Context, r ociregistry.Interface) error {
			rd, err := r.GetTag(ctx, "foo/read", "sometag")
			if rd != nil {
				rd.Close()
			}
			return err
		})
		yield1(ResolveBlob, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.ResolveBlob(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			return err
		})
		yield1(ResolveManifest, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.ResolveManifest(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			return err
		})
		yield1(ResolveTag, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.ResolveTag(ctx, "foo/read", "sometag")
			return err
		})
		yield1(PushBlob, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.PushBlob(ctx, "foo/write", ociregistry.Descriptor{
				MediaType: "application/json",
				Digest:    "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				Size:      3,
			}, strings.NewReader("foo"))
			return err
		})
		yield1(PushBlobChunked, func(ctx context.Context, r ociregistry.Interface) error {
			w, err := r.PushBlobChunked(ctx, "foo/write", 0)
			if err != nil {
				return err
			}
			w.Close()
			return nil
		})
		yield1(PushBlobChunkedResume, func(ctx context.Context, r ociregistry.Interface) error {
			w, err := r.PushBlobChunkedResume(ctx, "foo/write", "/someid", 3, 0)
			if err != nil {
				return err
			}
			data := []byte("some data")
			if _, err := w.Write(data); err != nil {
				return err
			}
			_, err = w.Commit(digest.FromBytes(data))
			return err
		})
		yield1(MountBlob, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.MountBlob(ctx, "foo/read", "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			return err
		})
		yield1(PushManifest, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := r.PushManifest(ctx, "foo/write", "sometag", []byte("something"), "application/json")
			return err
		})
		yield1(DeleteBlob, func(ctx context.Context, r ociregistry.Interface) error {
			return r.DeleteBlob(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		})
		yield1(DeleteManifest, func(ctx context.Context, r ociregistry.Interface) error {
			return r.DeleteManifest(ctx, "foo/write", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		})
		yield1(DeleteTag, func(ctx context.Context, r ociregistry.Interface) error {
			return r.DeleteTag(ctx, "foo/write", "sometag")
		})
		yield1(Repositories, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := ociregistry.All(r.Repositories(ctx, ""))
			return err
		})
		yield1(Tags, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := ociregistry.All(r.Tags(ctx, "foo/read", ""))
			return err
		})
		yield1(Referrers, func(ctx context.Context, r ociregistry.Interface) error {
			_, err := ociregistry.All(r.Referrers(ctx, "foo/read", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ""))
			return err
		})
	}
}
