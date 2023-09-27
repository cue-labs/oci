// package repo contains data values that are common to all CUE configurations
// in this repo. The list of configurations includes GitHub workflows, but also
// things like gerrit configuration etc.
package repo

import (
	"cuelabs.dev/go/oci/internal/ci/base"
)

base

githubRepositoryPath: "cue-labs/oci"

botGitHubUser:      "porcuepine"
botGitHubUserEmail: "cue.porcuepine@gmail.com"

defaultBranch: "main"

linuxMachine: "ubuntu-22.04"

latestGo: "1.21.x"
