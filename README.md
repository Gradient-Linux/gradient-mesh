# gradient-mesh

`gradient-mesh` is the fleet discovery daemon for Gradient Linux.

It advertises the local machine on LAN using mDNS, maintains an in-memory peer
list, and serves fleet state over a Unix socket at `/run/gradient/mesh.sock`.

## What is scaffolded here

- Persistent visibility config at `~/gradient/config/mesh.json`
- Thread-safe peer list management
- Unix socket query protocol for self and fleet snapshots
- A daemon loop shell with mDNS interfaces behind mocks
- A systemd unit for boot-time startup

## Layout

- `main.go` starts the daemon
- `internal/mesh/` contains config, peer list, socket, mDNS, and daemon code
- `scripts/gradient-mesh.service` is the systemd unit

## Build

```bash
go build ./...
```

## Test

```bash
go test ./...
```

## Runtime files

- Config: `~/gradient/config/mesh.json`
- Socket: `/run/gradient/mesh.sock`

