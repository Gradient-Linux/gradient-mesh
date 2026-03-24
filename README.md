# gradient-mesh

Peer-to-peer fleet discovery for Gradient Linux nodes on the local network.

## What it does

`gradient-mesh` advertises a machine over mDNS, discovers nearby Gradient Linux peers, tracks their last-seen state, and exposes fleet information over `/run/gradient/mesh.sock`. `concave` reads that local socket to power node visibility and fleet status commands without introducing a central coordinator.

## Requirements

- Ubuntu 24.04 LTS
- Go 1.25+
- Multicast DNS available on the local network

## Status

`gradient-mesh` is in development for Gradient Linux v0.3. The repository already contains the daemon shell, socket server, peer list, config handling, and mDNS wiring. Production rollout and broader fleet policy work are still in progress.

## Architecture

`gradient-mesh` is a local daemon. It has no remote control plane and no master node. Each machine publishes its own snapshot, listens for peers, and serves the current fleet view to `concave` over a Unix socket.

## Development

### Prerequisites

Install Go 1.25 or newer. mDNS-capable networking is only required for live discovery tests.

### Build

```bash
go build -o gradient-mesh .
```

### Test

```bash
go test ./...
```

### Run locally

```bash
./gradient-mesh run
./gradient-mesh version
./gradient-mesh help
```

### Repo layout

```text
gradient-mesh/
  internal/mesh/   daemon, peer list, socket, mDNS, config
  scripts/         systemd unit file
  docs/            repository docs
  main.go          CLI entrypoint
```

## Roadmap

The current line focuses on v0.3 LAN discovery and local fleet visibility. Later work adds stronger topology metadata, richer visibility controls, and deeper resolver-aware fleet status.

## License

License terms have not been published in this repository yet.
