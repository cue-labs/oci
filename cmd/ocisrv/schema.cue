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
	kind: "debug"
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
