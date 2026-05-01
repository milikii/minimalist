package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLogsCallsJournalctlWithDefaultSnapshot(t *testing.T) {
	app, _ := newTestApp(t)
	var called []string
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return "fake log line", "", nil
		},
	}

	if err := app.Logs(LogOptions{}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(called) != 1 {
		t.Fatalf("expected one call, got %v", called)
	}
	if !strings.Contains(called[0], "journalctl") || !strings.Contains(called[0], "-u minimalist.service") || !strings.Contains(called[0], "-n 50") {
		t.Fatalf("unexpected journalctl call: %s", called[0])
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "fake log line") {
		t.Fatalf("expected log output, got %q", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestLogsTargetsMihomoAndSupportsFilters(t *testing.T) {
	app, _ := newTestApp(t)
	var called []string
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return "", "", nil
		},
	}

	if err := app.Logs(LogOptions{Target: "mihomo", Lines: 20, Errors: true, Since: "15 minutes ago"}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	got := called[0]
	for _, needle := range []string{"-n 20", "-p warning", "-t mihomo-core", "--since 15 minutes ago"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("missing %q in journalctl call: %s", needle, got)
		}
	}
}

func TestLogsReturnsActionableError(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", errors.New("journalctl unavailable")
		},
	}

	err := app.Logs(LogOptions{Lines: 10})
	if err == nil || !strings.Contains(err.Error(), "问题: 日志读取失败") || !strings.Contains(err.Error(), "下一步: journalctl") || !strings.Contains(err.Error(), "文档: docs/README_FLOWS.md") {
		t.Fatalf("expected actionable error, got %v", err)
	}
}

func TestLogsReturnsActionableTimeoutError(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", errors.New("command timed out: journalctl -u minimalist.service -n 50 --no-pager")
		},
	}

	err := app.Logs(LogOptions{})
	if err == nil || !strings.Contains(err.Error(), "问题: 日志读取失败") || !strings.Contains(err.Error(), "command timed out") {
		t.Fatalf("expected actionable timeout error, got %v", err)
	}
}
