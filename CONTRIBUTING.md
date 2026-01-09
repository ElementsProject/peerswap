# Contributing to PeerSwap

Thanks for your interest in contributing to PeerSwap!

PeerSwap is software that interacts with real funds (Lightning + on-chain). Even
small changes can have safety, reliability, and security implications. Please
keep changes focused, test them well, and prefer incremental PRs.

## Quick Start (Dev Environment)

PeerSwap supports multiple development setups:

- **Nix (recommended):** `direnv allow` (or `nix develop`)
- **Devcontainer:** open the repo in VS Code and “Reopen in Container”

See `README.md` and `docs/setup-nix.md` for details.

## Substantial contributions only

We welcome contributions of all sizes, including small fixes. At the same time,
review bandwidth is limited and we sometimes receive spam/trivial PRs.

To keep review time focused on meaningful improvements:

- **Please avoid PRs that only do mechanical churn** (whitespace-only changes,
  mass reformatting, renames without behavior change, “update year in LICENSE”,
  etc.) unless there is a clear technical reason.
- **Small fixes are welcome when they matter** (wrong command in docs, broken
  link, misleading text, user-facing typo, flaky test fix, small bug fix).
  If possible, **bundle multiple small doc/typo fixes into a single PR**.
- If you are unsure whether a change is worth a PR, **open an issue first** or
  ask in the community channels (see `README.md`).

This isn’t meant to discourage newcomers — it’s only to reduce low-signal PRs
that are expensive to review.

## Before You Start

- For **larger changes** (new features, protocol changes, DB migrations, major
  refactors), please **open an issue or start a discussion first**. This helps
  avoid duplicate work and ensures the approach matches the project direction.
- Keep in mind PeerSwap has two main integration modes (**CLN plugin** and
  **LND daemon/CLI**). If your change affects one side, consider whether it also
  impacts the other.

## Development Practices

### Keep PRs Reviewable

- Prefer **small, focused PRs** over sweeping changes.
- Avoid mixing unrelated changes (e.g., refactor + behavior change + vendoring).
- Add/adjust tests when fixing bugs or changing behavior.

### Formatting

- Go code should be `gofmt`’d: `make fmt`

### Tests

- Unit tests (fast, local): `make test`
- Integration tests live under `./test` and can be run via the `test-matrix-*`
  targets in `Makefile` (these are heavier and may require external services).

### Linting

- Run: `make lint`
- Auto-fix where possible: `make lint/fix`

Note: `make lint` is configured to lint only changes vs the base branch when
possible (see `Makefile` for details).

### Protobuf / RPC Changes

If you change `.proto` files or RPC interfaces:

- Regenerate stubs: `make all-rpc` (uses `buf generate`)
- Lint/format protos: `make buf-lint` / `make buf-format`

### Documentation Diagrams

Some state/sequence diagrams are generated (Mermaid):

- Generate docs: `make all-docs`

## Code Review Expectations

- CI should be green, or the PR description should explain why not.
- Reviewers may ask for changes; please be responsive and keep iterations small.
- Maintainers may request splitting large PRs into smaller, independent pieces.

## Security

If you believe you found a **security vulnerability**, please **do not** open a
public issue. Prefer responsible disclosure using GitHub Security Advisories
for this repository.

## Licensing

Unless stated otherwise, contributions are accepted under the terms of the
repository license (see `LICENSE`).
