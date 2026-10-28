package main

// End-to-end tests of the CLI contract (P-01, P-02, CORE-01.5): they build the
// real binary and run it in a subprocess. No test touches the network.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hipoteca-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	binPath, err = buildBinary(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// buildBinary compiles the current package into dir and returns the executable
// path. The -o name carries .exe on Windows, where an extensionless file cannot
// be executed.
func buildBinary(dir string) (string, error) {
	pkg, err := build.Default.ImportDir(".", 0)
	if err != nil {
		return "", err
	}
	name := "hipoteca-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, pkg.Dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("built binary not found in %s: %w", dir, err)
	}
	return out, nil
}

func run(t *testing.T, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exitErr.ProcessState.ExitCode()
	}
	return out.Bytes(), errBuf.Bytes(), code
}

// exitCodeFor carries the §6.4 exit decision, extracted from main so it can be
// unit-tested: an interruption outranks whatever error the command returned and
// yields 130 without an envelope; a contract error yields its envelope and its
// stable exit code; success yields 0 with no envelope.
func TestExitCodeFor(t *testing.T) {
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	t.Run("cancelled context yields 130 with no envelope", func(t *testing.T) {
		code, env := exitCodeFor(cancelled(), nil)
		if code != errs.CodeInterrupted {
			t.Errorf("code = %d, want %d", code, errs.CodeInterrupted)
		}
		if env != nil {
			t.Errorf("an interruption never carries an envelope, got %+v", env)
		}
	})

	t.Run("interruption outranks the command error", func(t *testing.T) {
		code, env := exitCodeFor(cancelled(), errs.New(errs.KindInternal, "fallo", ""))
		if code != errs.CodeInterrupted {
			t.Errorf("code = %d, want %d", code, errs.CodeInterrupted)
		}
		if env != nil {
			t.Errorf("an interruption never carries an envelope, got %+v", env)
		}
	})

	t.Run("contract error yields its envelope and exit code", func(t *testing.T) {
		code, env := exitCodeFor(context.Background(), errs.New(errs.KindUsage, "uso", "pista"))
		if code != errs.ExitCode(errs.KindUsage) {
			t.Errorf("code = %d, want %d", code, errs.ExitCode(errs.KindUsage))
		}
		if env == nil {
			t.Fatal("a contract error must carry its envelope, got nil")
		}
		if env.Error.Type != string(errs.KindUsage) || env.Error.Message != "uso" || env.Error.Hint != "pista" {
			t.Errorf("unexpected envelope: %+v", env.Error)
		}
	})

	t.Run("error without contract kind falls into internal code 1", func(t *testing.T) {
		code, env := exitCodeFor(context.Background(), errors.New("boom"))
		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if env == nil {
			t.Fatal("an unexpected error must still carry an envelope, got nil")
		}
		if env.Error.Type != string(errs.KindInternal) {
			t.Errorf("type = %q, want internal", env.Error.Type)
		}
	})

	t.Run("success yields 0 with no envelope", func(t *testing.T) {
		code, env := exitCodeFor(context.Background(), nil)
		if code != 0 {
			t.Errorf("code = %d, want 0", code)
		}
		if env != nil {
			t.Errorf("success never carries an envelope, got %+v", env)
		}
	})
}

// P-01: `version` writes exactly one JSON value plus its newline to stdout and
// nothing else.
func TestVersionWritesExactlyOneJSONValue(t *testing.T) {
	stdout, stderr, code := run(t, "version")
	if code != 0 {
		t.Fatalf("version exited %d, stderr: %s", code, stderr)
	}
	if len(stderr) != 0 {
		t.Errorf("stderr must stay empty on success, got %q", stderr)
	}
	if !bytes.HasSuffix(stdout, []byte("\n")) {
		t.Errorf("stdout must end with a newline, got %q", stdout)
	}
	if bytes.HasSuffix(bytes.TrimRight(stdout, "\n"), []byte("\n")) {
		t.Errorf("exactly one JSON value allowed, got %q", stdout)
	}
	dec := json.NewDecoder(bytes.NewReader(stdout))
	var got struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
	}
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("stdout is not JSON: %v (raw: %q)", err, stdout)
	}
	if dec.More() {
		t.Errorf("stdout carries more than one JSON value: %q", stdout)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
}

// §6.4: an invalid command leaves stdout empty, writes the JSON envelope to
// stderr and exits with code 2 (usage).
func TestInvalidCommandEnvelopeOnStderrAndExit2(t *testing.T) {
	stdout, stderr, code := run(t, "inventado")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stderr: %s)", code, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must stay empty on failure, got %q", stdout)
	}
	var envelope struct {
		Error struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			ExitCode int    `json:"exit_code"`
			Hint     string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (raw: %q)", err, stderr)
	}
	if envelope.Error.Type != "usage" {
		t.Errorf("type = %q, want usage", envelope.Error.Type)
	}
	if envelope.Error.ExitCode != 2 {
		t.Errorf("exit_code = %d, want 2", envelope.Error.ExitCode)
	}
	if envelope.Error.Message == "" {
		t.Error("message must never be empty")
	}
}

// An unknown flag is also a usage error with the same contract: empty stdout,
// JSON envelope on stderr, exit code 2.
func TestUnknownFlagEnvelopeOnStderrAndExit2(t *testing.T) {
	stdout, stderr, code := run(t, "version", "--bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stderr: %s)", code, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must stay empty on failure, got %q", stdout)
	}
	var envelope struct {
		Error struct {
			Type     string `json:"type"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (raw: %q)", err, stderr)
	}
	if envelope.Error.Type != "usage" || envelope.Error.ExitCode != 2 {
		t.Errorf("unexpected envelope: %+v", envelope)
	}
}

// Review finding 3 (decision 7): explicit help is the documented exception to
// "stdout carries JSON" — prose for a human, exit 0, empty stderr.
func TestExplicitHelpIsProseOnStdoutWithExit0(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"version", "--help"}} {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := run(t, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
			}
			if len(stdout) == 0 {
				t.Error("the help prose must go to stdout, got nothing")
			}
			if json.Valid(stdout) {
				t.Errorf("the help must be prose, not a JSON value: %q", stdout)
			}
			if len(stderr) != 0 {
				t.Errorf("stderr must stay empty, got %q", stderr)
			}
		})
	}
}

// Review finding 4: `version extra` must be classified as usage (exit 2, type
// "usage"), not fall through to internal (exit 1).
func TestVersionExtraArgExits2WithUsageEnvelope(t *testing.T) {
	stdout, stderr, code := run(t, "version", "extra")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stderr: %s)", code, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must stay empty on failure, got %q", stdout)
	}
	var envelope struct {
		Error struct {
			Type     string `json:"type"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (raw: %q)", err, stderr)
	}
	if envelope.Error.Type != "usage" || envelope.Error.ExitCode != 2 {
		t.Errorf("unexpected envelope: %+v", envelope)
	}
}

// Review finding 5: with no subcommand stderr carries exactly one parseable
// JSON value — the envelope, with the hint to run `hipoteca --help` — and never
// the whole help text concatenated before it. stdout stays empty.
func TestBareInvocationWritesSingleJSONEnvelopeOnStderr(t *testing.T) {
	stdout, stderr, code := run(t)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stderr: %s)", code, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("stdout must stay empty on failure, got %q", stdout)
	}
	dec := json.NewDecoder(bytes.NewReader(stderr))
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := dec.Decode(&envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (raw: %q)", err, stderr)
	}
	if dec.More() {
		t.Errorf("stderr must carry exactly one JSON value, got %q", stderr)
	}
	if envelope.Error.Type != "usage" {
		t.Errorf("type = %q, want usage", envelope.Error.Type)
	}
	if !strings.Contains(envelope.Error.Hint, "--help") {
		t.Errorf("the hint must point at \"hipoteca --help\", got %q", envelope.Error.Hint)
	}
	if strings.Contains(string(stderr), "Usage:") || strings.Contains(string(stderr), "hipoteca lee los tipos") {
		t.Errorf("the help prose must never reach stderr with the envelope, got %q", stderr)
	}
}
