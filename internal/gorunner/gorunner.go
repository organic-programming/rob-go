package gorunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Result is the normalized output of a go toolchain subprocess call.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Elapsed  float64 // seconds
}

// TestEvent models one row from `go test -json` output.
type TestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// Run executes `go <subcommand> <args...>` with workdir, env, and timeout.
func Run(subcommand string, args []string, workdir string, env []string, timeoutS int) Result {
	ctx := context.Background()
	cancel := func() {}
	if timeoutS > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	}
	defer cancel()

	if workdir == "" {
		workdir = "."
	}

	argv := append([]string{subcommand}, args...)
	cmd := exec.CommandContext(ctx, "go", argv...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), env...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Elapsed: time.Since(started).Seconds(),
	}

	if err == nil {
		return res
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.ExitCode = 124
		if strings.TrimSpace(res.Stderr) == "" {
			res.Stderr = fmt.Sprintf("go %s timed out after %ds", subcommand, timeoutS)
		}
		return res
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res
	}

	res.ExitCode = 1
	if strings.TrimSpace(res.Stderr) == "" {
		res.Stderr = err.Error()
	}
	return res
}

// RunJSON executes `go test -json <args...>` and parses the event stream.
func RunJSON(args []string, workdir string, env []string, timeoutS int) (Result, []TestEvent) {
	jsonArgs := ensureJSON(args)
	res := Run("test", jsonArgs, workdir, env, timeoutS)

	dec := json.NewDecoder(strings.NewReader(res.Stdout))
	events := make([]TestEvent, 0)
	for {
		var ev TestEvent
		if err := dec.Decode(&ev); err != nil {
			break
		}
		events = append(events, ev)
	}

	return res, events
}

func ensureJSON(args []string) []string {
	for _, arg := range args {
		if arg == "-json" {
			return append([]string{}, args...)
		}
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, "-json")
	out = append(out, args...)
	return out
}
