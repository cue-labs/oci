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

package ocisrv

import "regexp"

#client: {
	kind:     "client"
	hostURL!: string
	debugID?: string
}

#select: {
	kind:      "select"
	registry!: #registry
	include?:  regexp.Valid
	exclude?:  regexp.Valid
}

#readOnly: {
	kind:      "readOnly"
	registry!: #registry
}

#immutable: {
	kind:      "immutable"
	registry!: #registry
}

#unify: {
	kind: "unify"
	registries!: [#registry, #registry]
}

#mem: {
	kind: "mem"
}

#debug: {
	kind:      "debug"
	registry!: #registry
}

#registry: #client |
	#select |
	#readOnly |
	#immutable |
	#unify |
	#mem |
	#debug

#registry: {
	kind!: string
}

registry!:   #registry
listenAddr!: string
