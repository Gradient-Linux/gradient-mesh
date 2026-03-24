# gradient-mesh docs

This directory tracks the public repository documentation for `gradient-mesh`.

## Start here

- [README.md](../README.md) explains the daemon boundary, runtime model, and local commands.
- [CONTRIBUTING.md](../CONTRIBUTING.md) covers build, test, and review expectations.

## Runtime contract

- Default socket: `/run/gradient/mesh.sock`
- Default mode: peer-to-peer LAN discovery over mDNS
- CLI entrypoints: `run`, `version`, `help`

## Scope

`gradient-mesh` owns node discovery and peer snapshots. It does not manage suites, resolve Python environments, or run notebooks.
