package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/rogpeppe/ociregistry/ociclient"
	"github.com/rogpeppe/ociregistry/ocidebug"
	"github.com/rogpeppe/ociregistry/ocimem"
	"github.com/rogpeppe/ociregistry/ociserver"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	fmt.Println("listening on http://localhost:5000")
	local := httptest.NewServer(ociserver.New(ocimem.New(), &ociserver.Options{
		DebugID: "direct",
	}))
	proxy := ociserver.New(ocidebug.New(ociclient.New(local.URL), logf), &ociserver.Options{
		DebugID: "proxy",
	})
	err := http.ListenAndServe(":5000", proxy)
	log.Fatal(err)
}

func logf(f string, a ...any) {
	log.Printf("ociclient: %s", fmt.Sprintf(f, a...))
}
