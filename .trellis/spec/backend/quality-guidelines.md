# Quality Guidelines

> Code quality standards for backend development.

## Overview

This project is a Go CLI/service manager. Quality gates are command-based and focused
on behavior, not style abstraction.

Required local checks:

```bash
env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

## Forbidden Patterns

- Adding operator commands in `internal/cli` without app-layer tests
- Hiding high-risk state drift behind "best effort" comments
- Reusing controller/network code in menu headers that must stay cheap
- Introducing follow/streaming UX on top of `Runner.Output()`
- Copying another menu loop instead of reusing a shared input helper

## Required Patterns

- Thin CLI dispatch, thick app orchestration
- Focused tests for every new operator command
- Cross-layer commands must define validation + rollback behavior
- Menu loops must treat `io.EOF` as a clean exit
- Operator-facing errors must be actionable

## Testing Requirements

For operator command changes:

1. Focused app tests
2. Focused CLI dispatch/help tests
3. Full package regression
4. If live runtime is touched, real smoke on:
   - `systemctl is-active`
   - `systemctl is-enabled`
   - `runtime-audit`
   - command surface being changed

### Good / Base / Bad cases

Every cross-layer operator feature must cover:
- Good: happy path
- Base: no-op or already-in-state path
- Bad: precondition failure
- Bad: downstream system failure
- Bad: rollback or timeout path if applicable

## Code Review Checklist

- Does the command mutate config truth and live state consistently?
- Are docs/help/parser/test names still aligned?
- Is the new helper actually shared, or just a renamed copy-paste?
- Does the menu stay usable when controller or stdin is unavailable?
- Did the author update `.trellis/spec/backend/*` when the command contract changed?
