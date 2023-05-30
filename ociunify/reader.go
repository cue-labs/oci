package ociunify

import (
	"context"
	"fmt"

	"go.cuelabs.dev/ociregistry"
)

// Reader methods.

func (u unifier) GetBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return runRead(ctx, u,
		func(ctx context.Context) t2[ociregistry.BlobReader] {
			return mk2(u.r0.GetBlob(ctx, repo, digest))
		},
		func(ctx context.Context) t2[ociregistry.BlobReader] {
			return mk2(u.r0.GetBlob(ctx, repo, digest))
		},
	).get()
}

func (u unifier) GetManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.BlobReader, error) {
	return runRead(ctx, u,
		func(ctx context.Context) t2[ociregistry.BlobReader] {
			return mk2(u.r0.GetManifest(ctx, repo, digest))
		},
		func(ctx context.Context) t2[ociregistry.BlobReader] {
			return mk2(u.r1.GetManifest(ctx, repo, digest))
		},
	).get()
}

func (u unifier) GetTag(ctx context.Context, repo string, tagName string) (ociregistry.BlobReader, error) {
	r0, r1 := both(
		func() t2[ociregistry.BlobReader] {
			return mk2(u.r0.GetTag(ctx, repo, tagName))
		},
		func() t2[ociregistry.BlobReader] {
			return mk2(u.r1.GetTag(ctx, repo, tagName))
		})
	switch {
	case r0.err == nil && r1.err == nil:
		if r0.x.Descriptor().Digest == r1.x.Descriptor().Digest {
			r1.x.Close()
			return r0.get()
		}
		r0.close()
		r1.close()
		return nil, fmt.Errorf("conflicting results for tag")
	case r0.err != nil && r1.err != nil:
		return r0.get()
	case r0.err == nil:
		return r0.get()
	case r1.err == nil:
		return r1.get()
	}
	panic("unreachable")
}

func (u unifier) ResolveBlob(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return runRead(ctx, u,
		func(ctx context.Context) t2[ociregistry.Descriptor] {
			return mk2(u.r0.ResolveBlob(ctx, repo, digest))
		},
		func(ctx context.Context) t2[ociregistry.Descriptor] {
			return mk2(u.r1.ResolveBlob(ctx, repo, digest))
		},
	).get()
}

func (u unifier) ResolveManifest(ctx context.Context, repo string, digest ociregistry.Digest) (ociregistry.Descriptor, error) {
	return runRead(ctx, u,
		func(ctx context.Context) t2[ociregistry.Descriptor] {
			return mk2(u.r0.ResolveManifest(ctx, repo, digest))
		},
		func(ctx context.Context) t2[ociregistry.Descriptor] {
			return mk2(u.r1.ResolveManifest(ctx, repo, digest))
		},
	).get()
}

func (u unifier) ResolveTag(ctx context.Context, repo string, tagName string) (ociregistry.Descriptor, error) {
	r0, r1 := both(
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r0.ResolveTag(ctx, repo, tagName))
		},
		func() t2[ociregistry.Descriptor] {
			return mk2(u.r1.ResolveTag(ctx, repo, tagName))
		},
	)
	switch {
	case r0.err == nil && r1.err == nil:
		if r0.x.Digest == r1.x.Digest {
			return r0.get()
		}
		return ociregistry.Descriptor{}, fmt.Errorf("conflicting results for tag")
	case r0.err != nil && r1.err != nil:
		return r0.get()
	case r0.err == nil:
		return r0.get()
	case r1.err == nil:
		return r1.get()
	}
	panic("unreachable")
}
