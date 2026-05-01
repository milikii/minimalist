# Directory Structure

> How backend code is organized in this project.

## Overview

This repository is a single Go CLI/service manager for `mihomo-core`.

There is no web server layer, no ORM layer, and no package split by feature domain.
Most operator-facing behavior lives in `internal/app`, while `internal/cli` is a thin
dispatch layer.

## Directory Layout

```text
cmd/minimalist/
  main.go                    # process entrypoint only

internal/app/
  app.go                     # service orchestration, menus, system side effects
  header.go                  # cheap local status snapshot and menu header rendering
  host_proxy.go              # operator-facing host OUTPUT proxy commands
  logs.go                    # operator-facing journal snapshot commands
  *_test.go                  # focused behavior tests

internal/cli/
  cli.go                     # top-level command parser and usage text
  cli_test.go                # dispatch and usage tests

internal/config/
  config.go                  # config truth at /etc/minimalist/config.yaml

internal/state/
  state.go                   # state truth at /var/lib/minimalist/state.json

internal/runtime/
  runtime.go                 # runtime config rendering and path layout

internal/system/
  system.go                  # command runner abstraction with timeout
```

## Module Organization

- `internal/cli` should only parse arguments and call `app.App` methods.
- `internal/app` owns orchestration and operator-facing workflows.
- `internal/config`, `internal/state`, `internal/runtime`, and `internal/system`
  are support layers and should not print operator UX text directly.

### Rule: new operator commands

For a new command such as `host-proxy` or `log`:

1. Add the service method in `internal/app/<topic>.go`
2. Add focused tests in `internal/app/<topic>_test.go`
3. Add CLI dispatch in `internal/cli/cli.go`
4. Add parser/help tests in `internal/cli/cli_test.go`

Do not put multi-step operator workflows directly in `internal/cli`.

## Naming Conventions

- Use lowercase snake-case file names: `host_proxy.go`, `logs.go`
- Use action-oriented method names on `App`:
  - `HostProxyStatus`
  - `HostProxyEnable`
  - `HostProxyDisable`
  - `Logs`
- Use helper names that describe behavior, not UI wording:
  - `statusSnapshot`
  - `readChoice`
  - `operatorActionError`

## Examples

- Thin CLI dispatch: `internal/cli/cli.go`
- Transactional operator action: `internal/app/host_proxy.go`
- Cheap menu-local read model: `internal/app/header.go`
- Focused operator tests: `internal/app/host_proxy_test.go`, `internal/app/logs_test.go`
