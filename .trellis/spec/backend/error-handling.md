# Error Handling

> How errors are handled in this project.

## Overview

This project mostly returns plain Go `error` values upward until the CLI or menu layer
decides how to present them.

For new operator-facing commands, raw low-level errors are not enough. The command must
return an actionable message that explains:

1. the operator-facing problem
2. the underlying cause
3. the next command to run
4. the relevant documentation path

Current helper: `internal/app/operator_error.go`

## Error Types

There are no custom typed error structs yet.

The project currently uses:
- plain sentinel-style errors such as `"没有启用的手动节点"`
- wrapped command failures from `internal/system.Runner`
- operator-facing formatted errors via `operatorActionError(...)`

## Operator Error Contract

### Format

All new operator-facing command errors must use this shape:

```text
问题: <operator-visible problem>; 原因: <underlying cause>; 下一步: <exact next command>; 文档: <doc-path>
```

### Required usage

Use the contract for commands that cross layers or mutate runtime state:
- `minimalist host-proxy status|enable|disable`
- `minimalist log ...`

Do not use the contract for low-level internal helpers that are not directly surfaced
to the operator.

## Validation and Error Matrix

### `minimalist host-proxy enable`

File paths:
- `internal/app/host_proxy.go`
- `internal/cli/cli.go`

Validation:
- requires root
- requires `cutover` ready
- requires at least one enabled manual node
- requires config save, render, and rule apply to succeed

| Case | Trigger | Expected behavior |
|---|---|---|
| Good | root + cutover ready + enabled manual node + apply succeeds | config truth flips on, runtime re-renders, rules apply, success message printed |
| Base | already on | no mutation, print already-on message |
| Bad | no enabled manual node | return actionable error, config truth unchanged |
| Bad | cutover blocked | return actionable error, config truth unchanged |
| Bad | render/apply failure | return actionable error, rollback config truth |
| Bad | rollback failure | return actionable error that explicitly says rollback failed |

### `minimalist log`

File paths:
- `internal/app/logs.go`
- `internal/cli/cli.go`

Validation:
- accepts `mihomo`
- accepts `--errors`
- accepts `-n|--lines <count>`
- accepts `--since <window>`
- rejects unknown args

| Case | Trigger | Expected behavior |
|---|---|---|
| Good | `minimalist log --lines 20` | `journalctl -u minimalist.service -n 20 --no-pager` |
| Good | `minimalist log mihomo --errors` | adds target filter and warning priority |
| Bad | unknown arg | parser error with exact unknown token |
| Bad | `journalctl` unavailable | actionable operator error |
| Bad | command timeout | actionable operator error |

## Required Tests

- `internal/app/host_proxy_test.go`
  - no enabled manual node
  - cutover blocked
  - apply failure rollback
  - render failure rollback
- `internal/app/logs_test.go`
  - default snapshot invocation
  - target/filter parsing behavior
  - missing `journalctl`
  - timeout behavior
- `internal/cli/cli_test.go`
  - help text includes new commands
  - usage errors for incomplete command lines
  - unknown arg / unknown subcommand behavior

## Common Mistakes

- Returning raw `iptables` / `journalctl` errors directly to users
- Mutating config truth before checking high-risk preconditions
- Reporting `stopped` when the real state is `unknown`
- Adding new command flags in docs but not in parser, or vice versa
