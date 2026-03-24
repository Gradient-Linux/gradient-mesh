# Contributing to gradient-mesh

## Scope

This repository only contains the mesh daemon and its tests.

## Workflow

1. Work on `feature/mesh`.
2. Keep changes isolated to this repository.
3. Add or update unit tests for any behavior change.
4. Run `go test ./...` and `go build ./...` before commit.

## Code rules

- Keep network behavior behind interfaces.
- Prefer small, composable functions.
- Use deterministic persistence and sorting in tests.
- Do not hardcode runtime paths outside the documented defaults.

