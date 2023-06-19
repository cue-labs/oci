package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	fmt.Println("listening on http://localhost:5000")
	local := httptest.NewServer(ociserver.New(ocimem.New(), &ociserver.Options{
		DebugID: "direct",
	}))
	proxy := ociserver.New(ociclient.New(local.URL, nil), &ociserver.Options{
		DebugID: "proxy",
	})
	err := http.ListenAndServe(":5000", proxy)
	log.Fatal(err)
}
