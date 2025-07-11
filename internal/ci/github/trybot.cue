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

			let runnerOSExpr = "runner.os"
			let runnerOSVal = "${{ \(runnerOSExpr) }}"
			let installGo = _repo.installGo & {
				#setupGo: with: "go-version": _repo.latestGo
				_
			}
			let _setupGoActionsCaches = _repo.setupGoActionsCaches & {
				#goVersion: _repo.latestGo
				#os:        runnerOSVal
				_
			}

			// Only run the trybot workflow if we have the trybot trailer, or
			// if we have no special trailers. Note this condition applies
			// after and in addition to the "on" condition above.
			if: "\(_repo.containsTrybotTrailer) || ! \(_repo.containsDispatchTrailer)"

			steps: [
				for v in _repo.checkoutCode {v},

				for v in installGo {v},
				for v in _setupGoActionsCaches {v},

				_repo.earlyChecks,

				for _, v in perModuleChecks {v},

				// Run "go work sync" after we have checked and tested every module.
				// This way, if a "go test" command fails, it is much easier for the developer
				// to reproduce on their machine without having to remember "go work sync".
				// If "go work sync" makes any changes, then the git clean check below will fail anyway.
				githubactions.#Step & {
					run: "go work sync"
				},

				_repo.checkGitClean,
			]
		}
	}

	let perModuleChecks = list.FlattenN([
		for _, goModPath in _repo.modules
		let modDir = path.Dir(goModPath)
		let modIsInternal = _#goModDirIsInternal & {#goModDir: modDir, _}
		for _, gowork in ["", if !modIsInternal {"off"}]
		let stepName = modDir + [if gowork != "" {" with GOWORK=\(gowork)"}, ""][0] {[
			[for step in [_#goGenerate, _#goTest, _#goCheck] {
				step & {
					#name: stepName
					"working-directory": modDir
					env: {
						GOWORK: gowork
					}
				}
			}],
			// Note: "uses" steps don't require or allow the other fields added above.
			[_#goStaticCheck & {
				#name: stepName
			}],
		]},
	], 2)

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

	_#goStaticCheck: githubactions.#Step & {
		#name: string
		name:  "Staticcheck \(#name)"
		// TODO(mvdan): once we can do 'go tool staticcheck' with Go 1.24+,
		// then using this action is probably no longer worthwhile.
		// Note that we should then persist staticcheck's cache too.
		uses: "dominikh/staticcheck-action@v1"
		with: {
			version:      "2025.1" // Pin a version for determinism.
			"install-go": false    // We install Go ourselves.
			"use-cache":  false    // We use a volume cache instead.
		}
	}
}
