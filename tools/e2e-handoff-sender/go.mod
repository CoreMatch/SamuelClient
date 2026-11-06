// Nested module placeholder: e2e-handoff-sender imports
// MatchCoreArena-Client/internal/interop, which Go's internal-package rule
// only resolves inside the MatchCoreArena-Client module. This go.mod keeps
// SamuelClient's `go build ./...` from seeing that import.
//
// To run the e2e driver: copy main.go (NOT this file) into
// <MatchCoreArena-Client>/tools/e2e-handoff-sender/ of a MCA client
// checkout and `go run ./tools/e2e-handoff-sender` there.
module e2e-handoff-sender-placeholder

go 1.26
