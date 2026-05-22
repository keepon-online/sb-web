// Package main is a placeholder for the scripts/ directory.
//
// The directory hosts standalone Go programs (test_api_endpoints.go,
// verify_database.go) that each declare `package main` with their own main()
// function and are guarded by the `//go:build ignore` build tag so they are
// only compiled via `go run scripts/<file>.go`. Without this placeholder
// `go build ./...` would report "function main is undeclared" because the
// directory would otherwise contain `package main` files but no main()
// reachable under the default build tags.
package main

func main() {}
