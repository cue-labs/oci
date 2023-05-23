package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/rogpeppe/ociregistry/ociclient"
	"github.com/rogpeppe/ociregistry/ocimem"
	"github.com/rogpeppe/ociregistry/ociserver"
)

func main() {
	fmt.Println("listening on http://localhost:5000")
	local := httptest.NewServer(ociserver.New(ocimem.New(), nil))
	proxy := ociserver.New(ociclient.New(local.URL), nil)
	err := http.ListenAndServe(":5000", proxy)
	log.Fatal(err)
}
