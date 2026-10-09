// Package integration holds Milestone 1 deployment verification tests.
//
// The tests carry the `integration` build tag and only run with:
//	go test -tags integration ./internal/integration/... -count=1
// They need a Docker daemon (disposable Postgres + real binary) and skip
// gracefully anywhere else.
package integration
