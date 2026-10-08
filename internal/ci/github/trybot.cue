// Copyright 2022 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"list"
	"path"

	"cue.dev/x/githubactions"
)

// The trybot workflow.
workflows: trybot: _repo.bashWorkflow & {
	name: _repo.trybot.name

	on: {
		push: {
			branches: list.Concat([[_repo.testDefaultBranch], _repo.protectedBranchPatterns]) // do not run PR branches
		}
		pull_request: {}
	}

	jobs: {
		test: {
			"runs-on": _repo.linuxMachine

			let installGo = _repo.installGo & {
				#setupGo: with: "go-version": _repo.latestGo
				_
			}

			// Only run the trybot workflow if we have the trybot trailer, or
			// if we have no special trailers. Note this condition applies
			// after and in addition to the "on" condition above.
			if: "\(_repo.containsTrybotTrailer) || ! \(_repo.containsDispatchTrailer)"

			steps: [
				for v in _repo.checkoutCode {v},
				for v in installGo {v},
				for v in _repo.setupCaches {v},

				_repo.earlyChecks,
				_#installStaticCheck,

				for _, v in perModuleChecks {v},

				// Run "go work sync" after we have checked and tested every module.
				// This way, if a "go test" command fails, it is much easier for the developer
				// to reproduce on their machine without having to remember "go work sync".
				// If "go work sync" makes any changes, then the git clean check below will fail anyway.
				{
					run: "go work sync"
				},

				_repo.checkGitClean,
			]
		}
	}

	let perModuleChecks = [
		for _, goModPath in _repo.modules
		let modDir = path.Dir(goModPath)
		let modIsInternal = _#goModDirIsInternal & {#goModDir: modDir, _}
		for _, gowork in ["", if !modIsInternal {"off"}]
		let stepName = modDir + [if gowork != "" {" with GOWORK=\(gowork)"}, ""][0]
		// Running staticcheck in workspace mode is enough; vet and the tests
		// already cover the dependency versions used with GOWORK=off.
		for step in [_#goGenerate, _#goTest, _#goCheck, if gowork == "" {_#goStaticCheck}] {
			step & {
				#name:               stepName
				"working-directory": modDir
				env: GOWORK: gowork
			}
		},
	]

	// _#goModIsInternal determins whether a repo root-relative directory path
	// to a go.mod filepath is internal from a Go modules perspective.
	_#goModDirIsInternal: {
		// #path is the repo root-relative directory path containing a go.mod
		// file
		#goModDir: string

		let pathElems = path.Split(#goModDir)
		let isInternal = [for _, v in pathElems {v == "internal"}]
		_res: [for i, v in isInternal {[if i == 0 {false}, _res[i-1]][0] || v}]

		// In case the root is a module
		*_res[0] | false
	}

	_#goGenerate: githubactions.#Step & {
		#name: string
		name:  "Generate \(#name)"
		run:   "go generate ./..."
	}

	_#goTest: githubactions.#Step & {
		#name: string
		name:  "Test \(#name)"
		run:   "go test ./..."
	}

	_#goCheck: githubactions.#Step & {
		// These checks can vary between platforms, as different code can be built
		// based on GOOS and GOARCH build tags.
		// However, CUE does not have any such build tags yet, and we don't use
		// dependencies that vary wildly between platforms.
		// For now, to save CI resources, just run the checks on one matrix job.
		// TODO: consider adding more checks as per https://github.com/golang/go/issues/42119.
		#name: string
		name:  "Check \(#name)"
		run:   "go vet ./..."
	}

	// _#installStaticCheck adds staticcheck to PATH, so that it can be run
	// on each module. We cannot use _repo.staticcheck, as -modfile is not
	// allowed in workspace mode. The tools module is not in go.work, so that
	// its dependencies stay out of the published modules.
	_#installStaticCheck: githubactions.#Step & {
		name: "Install staticcheck"
		run: """
			echo "$(dirname "$(GOWORK=off go -C internal/tools tool -n staticcheck)")" >> "$GITHUB_PATH"
			"""
	}

	_#goStaticCheck: githubactions.#Step & {
		#name: string
		name:  "Staticcheck \(#name)"
		env:   _repo.staticcheck.env
		run:   "staticcheck ./..."
	}
}
