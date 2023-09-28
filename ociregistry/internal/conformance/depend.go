// Copyright 2023 CUE Labs AG
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

//go:build donotincludeme

// Note: this file is here so we have the conformance package as part
// of our dependencies, so we're sure to be able to run `go test` on it.
// We can't import it from conformance_test.go because it has init-time
// functions that fail if the correct env vars aren't set.

package conformance

import _ "github.com/opencontainers/distribution-spec/conformance"
