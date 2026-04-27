package system

import (
	"strings"
	"testing"
	"time"
)

func TestOutputSuccess(t *testing.T) {
	runner := Runner{Timeout: 2 * time.Second}
	stdout, stderr, err := runner.Output("bash", "-lc", "printf 'hello'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestOutputFailureUsesStderr(t *testing.T) {
	runner := Runner{Timeout: 2 * time.Second}
	_, _, err := runner.Output("bash", "-lc", "echo boom >&2; exit 7")
	if err == nil {
		t.Fatalf("expected failure")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected stderr in error, got: %v", err)
	}
}

func TestOutputFailureWithoutStderrUsesExecError(t *testing.T) {
	runner := Runner{Timeout: 2 * time.Second}
	_, _, err := runner.Output("bash", "-lc", "exit 7")
	if err == nil {
		t.Fatalf("expected failure")
	}
	if !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("expected exec error in message, got: %v", err)
	}
}

func TestOutputTimeout(t *testing.T) {
	runner := Runner{Timeout: 10 * time.Millisecond}
	_, _, err := runner.Output("bash", "-lc", "sleep 1")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout wording, got: %v", err)
	}
}

func TestRunDelegatesToOutputAndReturnsFailure(t *testing.T) {
	runner := Runner{Timeout: 2 * time.Second}
	err := runner.Run("bash", "-lc", "echo run-fail >&2; exit 3")
	if err == nil {
		t.Fatalf("expected run failure")
	}
	if !strings.Contains(err.Error(), "run-fail") {
		t.Fatalf("expected stderr in run error, got: %v", err)
	}
}

func TestOutputUsesDefaultTimeoutWhenZero(t *testing.T) {
	runner := Runner{}
	stdout, _, err := runner.Output("bash", "-lc", "printf 'default-timeout'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "default-timeout" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestNewRunnerSetsDefaultTimeout(t *testing.T) {
	runner := NewRunner()
	if runner.Timeout != 30*time.Second {
		t.Fatalf("expected default timeout, got %s", runner.Timeout)
	}
}
