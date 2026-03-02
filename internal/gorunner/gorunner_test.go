package gorunner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	result := Run("version", nil, ".", nil, 10)
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "go1.") {
		t.Fatalf("stdout=%q, want go1.*", result.Stdout)
	}
}

func TestRunEnv(t *testing.T) {
	result := Run("env", []string{"GOOS"}, ".", nil, 10)
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) == "" {
		t.Fatal("expected non-empty stdout")
	}
}

func TestRunBadDir(t *testing.T) {
	result := Run("version", nil, filepath.Join(t.TempDir(), "missing"), nil, 5)
	if result.ExitCode == 0 {
		t.Fatalf("expected non-zero exit, got 0 stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
}

func TestRunTimeout(t *testing.T) {
	dir := writeTempModule(t, map[string]string{
		"go.mod": "module example.com/timeout\n\ngo 1.24.0\n",
		"slow_test.go": `package slow

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T) {
	time.Sleep(2 * time.Second)
}
`,
	})

	result := Run("test", []string{"./..."}, dir, nil, 1)
	if result.ExitCode == 0 {
		t.Fatalf("expected timeout failure, got stdout=%q", result.Stdout)
	}
	if result.ExitCode != 124 {
		t.Fatalf("exit=%d, want 124", result.ExitCode)
	}
}

func TestRunJSON(t *testing.T) {
	dir := writeTempModule(t, map[string]string{
		"go.mod": "module example.com/json\n\ngo 1.24.0\n",
		"main_test.go": `package jsonmod

import "testing"

func TestPass(t *testing.T) {}
`,
	})

	result, events := RunJSON([]string{"./..."}, dir, nil, 20)
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", result.ExitCode, result.Stderr, result.Stdout)
	}
	if len(events) == 0 {
		t.Fatalf("expected parsed events, stdout=%q", result.Stdout)
	}

	foundPass := false
	for _, ev := range events {
		if ev.Action == "pass" && ev.Test == "TestPass" {
			foundPass = true
			break
		}
	}
	if !foundPass {
		t.Fatalf("did not find pass event for TestPass; events=%+v", events)
	}
}

func writeTempModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if runtime.GOOS == "windows" {
		// Keep file creation explicit for debugging path handling on Windows.
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			t.Fatalf("go.mod missing: %v", err)
		}
	}

	return dir
}
