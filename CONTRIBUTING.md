# Contributing to gradient-mesh

Contributions are welcome for peer discovery, socket behavior, persistence, tests, and documentation. Keep changes focused on mesh discovery and node state. Suite management, resolver logic, and notebook orchestration belong in other repositories.

## Before you start

Read [README.md](README.md) and [docs/README.md](docs/README.md) before changing the daemon contract or peer data model.

## Development setup

Use Ubuntu 24.04 with Go 1.25 or newer.

```bash
git clone <repo-url>
cd gradient-mesh
go build -o gradient-mesh .
go test ./...
./gradient-mesh help
```

## Making changes

### Branching

Use one of these branch prefixes:

- `feat/<slug>`
- `fix/<slug>`
- `docs/<slug>`

### Commit messages

Format commits as `<type>(<scope>): <summary>`.

Use these types:

- `feat`
- `fix`
- `refactor`
- `test`
- `docs`
- `chore`

Keep the summary under 72 characters.

Examples:

- `feat(mesh): add peer expiry jitter`
- `fix(socket): handle missing workspace state`
- `docs(readme): clarify discovery requirements`

### Tests

- Add or update unit tests for new functions and behavior changes.
- Run `go test ./...` before opening a pull request.
- Run `go test -race ./...` when you touch shared state or long-lived goroutines.
- Keep network behavior mockable. Unit tests must not rely on a live LAN.

### Pull requests

- Keep pull requests narrow and easy to review.
- Explain the runtime effect of socket, peer-list, or mDNS changes.
- Include manual verification steps when behavior depends on live discovery.

## Code conventions

- Keep the daemon peer-to-peer. Do not add a central coordinator.
- Preserve `/run/gradient/mesh.sock` as the default socket path unless compatibility requires a documented change.
- Keep peer state deterministic and stable for tests.
- Hide multicast and network behavior behind interfaces where practical.
- Use separate command arguments instead of shell interpolation.

## What we don't accept

- Dependencies added without prior discussion in an issue.
- Code that assumes a single network environment or hardcodes site-specific addresses.
- Shell string interpolation with user-controlled input.
- Business logic copied in from `concave`.

## License

By contributing, you agree that your contributions will be released under the repository license when one is published.
