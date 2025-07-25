// package repo contains data values that are common to all CUE configurations
// in this repo. The list of configurations includes GitHub workflows, but also
// things like gerrit configuration etc.
package repo

import (
	"github.com/cue-lang/tmp/internal/ci/base"
)

base

githubRepositoryPath: "cue-labs/oci"

botGitHubUser:      "porcuepine"
botGitHubUserEmail: "cue.porcuepine@gmail.com"

defaultBranch: "main"

latestGo: "1.24.x"

// modules is a list of Unix paths of go.mod files for go modules in this
// repository
modules: [...string]
modules: [
	"./cmd/ocisrv/go.mod",
	"./ociregistry/internal/conformance/go.mod",
	"./ociregistry/go.mod",
	"./internal/ci/go.mod",
]
