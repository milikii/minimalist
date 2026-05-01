# Logging Guidelines

> How logging is done in this project.

## Overview

This repository does not use a structured Go logger for operator workflows.

There are two distinct logging surfaces:

1. service/runtime logs emitted by `mihomo-core` into `journalctl`
2. operator-facing CLI/menu output printed via `Stdout` / `Stderr`

Do not mix them.

## Operator Log Command

Current snapshot command:

```text
minimalist log [mihomo] [--errors] [-n|--lines <count>] [--since <window>]
```

Implementation files:
- `internal/app/logs.go`
- `internal/cli/cli.go`

### Scope rules

- Snapshot only. No `-f` follow mode unless `internal/system` grows a streaming API.
- Default target is `minimalist.service`
- `mihomo` switches to `mihomo-core` tag filtering
- Default line count is `50`

## Log Levels

For the operator CLI:
- success and normal snapshots go to `Stdout`
- actionable failures return an `error`
- internal helpers should not print directly unless they are explicitly rendering output

For `journalctl` filtering:
- `--errors` currently maps to warning-and-above filtering
- do not claim "errors only" if the actual filter includes warnings

## What to Log

- recent service state for debugging
- warning/error windows via `runtime-audit`
- targeted `mihomo-core` lines when operator asks for them

## What NOT to Log

- controller secret
- raw credentials inside node URLs
- unnecessary duplicate copies of the same command failure

## Required Tests

- default snapshot uses `minimalist.service`
- `mihomo` target path is correct
- explicit line count is honored
- unavailable/timeout paths produce actionable operator errors

## Common Mistakes

- designing CLI syntax that docs use but parser does not accept
- pretending follow mode exists on top of a non-streaming runner
- logging secrets or full subscription payloads
