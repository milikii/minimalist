# Menu CLI Redesign Test Plan

Date: 2026-05-01
Source plan: [docs/superpowers/plans/2026-04-30-menu-cli-redesign.md](/home/projects/minimalist/docs/superpowers/plans/2026-04-30-menu-cli-redesign.md:1)
Review mode: `/autoplan`

## Goal

Lock the menu and CLI redesign around safe operator flows, not cosmetic parity.

## Critical flows

1. Header rendering stays fast when controller is down.
2. Menu loops exit cleanly on EOF and invalid input.
3. `host-proxy` toggles do not leave config truth and live iptables state split.
4. `log` works in snapshot mode and degrades cleanly when `journalctl` is unavailable.
5. Dangerous operations are confirmed and cancellation is explicit.
6. Diagnostics remain reachable after top-level menu reshuffle.

## Required test matrix

### Header

- `renderStatusHeader` returns within the local runner timeout when controller is unreachable.
- Header distinguishes `running/stopped/unknown` instead of collapsing all failures to `stopped`.
- Header distinguishes node readiness `none/partial/ready`.

### Menu control flow

- `Menu()` exits on `io.EOF` without looping forever.
- `nodesMenu/subscriptionsMenu/...` reuse one shared `*bufio.Reader`.
- List actions stay in submenu after success.
- Error actions print feedback once and keep the submenu usable.

### Host proxy

- `HostProxyEnable` rolls back config when `RenderConfig` fails.
- `HostProxyEnable` rolls back config when `ApplyRules` fails.
- `HostProxyEnable` refuses to mutate config when `ensureCutoverReady` fails.
- `HostProxyEnable` refuses to mutate config when no enabled manual node exists.
- `HostProxyDisable` restores safe default and re-applies rules.
- `host-proxy status` reports config truth without requiring controller access.

### Logs

- `log` snapshot mode supports explicit line count.
- `log` unknown arg path returns actionable usage.
- `log` missing `journalctl` path reports problem + next step.
- `log` timeout path reports partial failure clearly.
- `log -f` must be removed or moved onto a streaming runner before implementation.

### Diagnostics IA

- Top-level menu still exposes a dedicated diagnostics surface containing `status`, `healthcheck`, and `runtime-audit`.
- High-risk install/cutover actions are not mixed with daily `start/stop/restart`.

### Docs

- One canonical operator flow page is updated.
- README quickstart links to the canonical operator flow page instead of duplicating steps.
- Alias `m` is documented as optional, not canonical.

## Current gaps

- No test currently covers menu EOF exit.
- No test currently covers transactional `host-proxy` rollback.
- No test currently covers `journalctl` streaming incompatibility.
- No test currently covers diagnostics discoverability after top-level reshuffle.

## Recommendation

Do not implement the full Phase 1-4 plan until:

1. `log -f` is either removed or backed by a streaming system runner.
2. `host-proxy` is redesigned as a transactional service method.
3. A dedicated diagnostics surface is restored.
