package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rogpeppe/ociregistry/ocimem"
	"github.com/rogpeppe/ociregistry/ociserver"
)

func main() {
	fmt.Println("listening on http://localhost:5000")
	err := http.ListenAndServe(":5000", ociserver.New(ocimem.New(), nil))
	log.Fatal(err)
}
