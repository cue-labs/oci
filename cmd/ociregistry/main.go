package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rogpeppe/ociregistry"
	"github.com/rogpeppe/ociregistry/ocifunc"
	"github.com/rogpeppe/ociregistry/ocimem"
	"github.com/rogpeppe/ociregistry/ociserver"
)

func main() {
	fmt.Println("listening on http://localhost:5000")
	err := http.ListenAndServe(":5000", ociserver.New(ocifunc.New(funcsFromRegistry(ocimem.New())), nil))
	log.Fatal(err)
}

func funcsFromRegistry(r ociregistry.Interface) ocifunc.Funcs {
	return ocifunc.Funcs{
		GetBlob:         r.GetBlob,
		GetManifest:     r.GetManifest,
		GetTag:          r.GetTag,
		ResolveBlob:     r.ResolveBlob,
		ResolveManifest: r.ResolveManifest,
		ResolveTag:      r.ResolveTag,
		PushBlob:        r.PushBlob,
		PushBlobChunked: r.PushBlobChunked,
		MountBlob:       r.MountBlob,
		PushManifest:    r.PushManifest,
		DeleteBlob:      r.DeleteBlob,
		DeleteManifest:  r.DeleteManifest,
		DeleteTag:       r.DeleteTag,
		Repositories:    r.Repositories,
		Tags:            r.Tags,
		Referrers:       r.Referrers,
	}
}
