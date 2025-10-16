# PeerSwap Agent Guide

## Quick Context
- PeerSwap balances Lightning Network channels through atomic swaps with direct peers.
- The Go 1.22+ codebase splits CLN and LND entrypoints under `cmd/` with shared logic in `swap/`, `lightning/`, and `onchain/`.
- Development relies on Nix (`nix develop`) for reproducible tooling; macOS builds run natively without Docker.

## Environment & Tooling
- Enter the dev shell with `nix develop` (fallback: `nix-shell`).
- Install helper binaries via `make tool`; outputs land in `tools/bin/`.
- Build local binaries with `make bins`; integration tests depend on artifacts in `out/`.
- Run all Go commands from the module root with modules enabled (`GO111MODULE=on`).

## Default Workflow
1. Confirm the target behaviour in `docs/peer-protocol.md` and existing handlers before editing.
2. Keep CLN/LND feature parity in mind and land changes as focused, reviewable commits.
3. Reuse abstractions in `swap/`, `lightning/`, `txwatcher/`, and friends instead of ad-hoc packages.
4. Encapsulate state mutations behind well-tested services; avoid globals.
5. Update docs whenever behaviour, flags, or RPC surfaces change.

## Linting & Formatting
- Always `gofmt` (or `goimports`) Go code before committing.
- Run `make lint` for any Go changes; handoff only when lint is clean.
- Keep newly created Markdown or config files ASCII unless prior content requires UTF-8.

## Testing Expectations
- Use the standard library `testing` package only—no third-party frameworks.
- Write table-driven tests when practical and cover both CLN and LND paths where relevant.
- Prefer deterministic fixtures; inject clocks (`clockwork`) instead of calling `time.Sleep`.

### go-cmp Wrapper Pattern
Keep `github.com/google/go-cmp/cmp` behind a helper so assertions stay consistent:
```go
package testutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func AssertDiff[T any](t *testing.T, want, got T, opts ...cmp.Option) {
	t.Helper()
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}
```
- Place helpers in the relevant `_test.go` file or a shared `testutil` package.
- Keep go-cmp out of production builds; limit imports to test-only files.

## Coding Style Highlights (Uber Go Guide)
- Keep package names short and avoid stutter (`swapiterator`, not `swapIterator`).
- Return early on errors; use `if err != nil` strictly for error handling.
- Accept `context.Context` as the first parameter for work that performs I/O or long-running tasks.
- Prefer constructors returning concrete types; depend on interfaces only at boundaries.
- Use `var` when zero values are desired and short declarations for obvious initialisations.
- Group related declarations and keep exported APIs minimal.

## Repository Landmarks
- `cmd/peerswap-plugin`: Core Lightning plugin entrypoint.
- `cmd/peerswaplnd`: LND daemon (`peerswapd`) and CLI (`pscli`).
- `swap/`: Swap orchestration logic, state machines, and persistence.
- `lightning/`: Lightning client adapters for CLN/LND.
- `onchain/`: Bitcoin and Liquid integration primitives.
- `test/`: Slow, tagged integration tests.
- `docs/`: Operator and contributor documentation.

## Before You Ship
- Run `make lint` for Go changes and resolve all warnings.
- Execute `go test ./... -tags dev,fast_test` when Go logic changes.
- Update documentation or configuration if user-facing behaviour shifts.
- Call out follow-up work or risk in commit messages or PR notes.

## Support
- Discord: https://discord.gg/wpNv3PG8G2 (community support and coordination).
- Track outstanding work in repository issues—avoid private TODO lists.
