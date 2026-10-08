// This module tracks tools used via `go tool` in CI, so that their
// dependencies stay out of the go.mod files of the published modules.
// It is not part of go.work, so use GOWORK=off with it.
module test/tools

go 1.26.0

tool honnef.co/go/tools/cmd/staticcheck

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/tools v0.44.1-0.20260420230617-19499e7caabc // indirect
	honnef.co/go/tools v0.8.1 // indirect
)
