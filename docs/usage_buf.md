# Usage Guide

In this project, we utilize Buf CLI to streamline the development environment for Protocol Buffers (protobuf).Buf is a powerful tool for managing the build, linting, formatting, and validation processes for protobuf files in a consistent and efficient manner.

- [Buf](https://buf.build/)
- [Buf Github URL](https://github.com/bufbuild/buf)

## Why We Use Buf

The adoption of Buf provides several advantages:
- Strict Lint Checks: Ensures a consistent code style across the team and helps prevent potential bugs.
- Simplified Build Process: Reduces the complexity of configuring protoc commands with multiple options.
- CI/CD Integration: Easily integrates automatic linting, formatting, and validation into the CI/CD pipeline.
- Module Management: Simplifies dependency management with Buf modules.

## Installation Instructions

Buf CLI setup guide see this [Install the Buf CLI](https://buf.build/docs/cli/installation/):

## Notes on commands

```bash
make buf-lint   ## lint
make buf-format ## format -w
```
