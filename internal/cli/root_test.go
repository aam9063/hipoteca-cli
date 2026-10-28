package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// Review finding 5: with no subcommand the only output is the usage error that
// main turns into the JSON envelope on stderr — never the whole help text, which
// would concatenate prose and JSON on stderr and break parsers. stdout stays
// empty (P-01) and the hint points at `hipoteca --help`.
func TestRootWithoutSubcommandKeepsStdoutEmpty(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("running hipoteca without a subcommand must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("the command must not print the help itself; the envelope is main's job, got %q", stderr.String())
	}
	if e.Hint == "" || !strings.Contains(e.Hint, "--help") {
		t.Errorf("the hint must point at \"hipoteca --help\", got %q", e.Hint)
	}
}

// Review finding 3 (decision 7): explicit help is prose for a human on stdout
// with exit 0 — the documented exception to "stdout carries JSON". The help is
// printed by cobra to the command's out writer; nothing reaches stderr.
func TestExplicitHelpWritesProseToStdout(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"version", "--help"}} {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("explicit help must succeed, got %v", err)
			}
			if stdout.Len() == 0 {
				t.Error("the help prose must go to stdout, got nothing")
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr must stay empty, got %q", stderr.String())
			}
		})
	}
}

// Review finding 4: `version extra` is a usage error of the contract (exit 2),
// not an unclassified error that main would report as internal (exit 1).
func TestVersionExtraArgIsAUsageError(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version", "extra"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("version must not accept extra arguments")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}

// An unknown command is a usage error of the contract (§6.4): it must reach main
// as an errs error with kind usage (so the process exits 2), and stdout must stay
// empty because the envelope goes to stderr.
func TestUnknownCommandIsAUsageError(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"inventado"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("an unknown command must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("unknown command kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}

// An unknown flag is also a usage error, with stdout empty.
func TestUnknownFlagIsAUsageError(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version", "--bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("an unknown flag must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("the error must carry a contract kind, got %v", err)
	}
	if e.Kind != errs.KindUsage {
		t.Errorf("unknown flag kind = %q, want usage", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}

// The completion command is not part of the CLI contract, so asking for it must
// fail like any other unknown command instead of writing a shell script.
func TestCompletionCommandIsDisabled(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"completion", "bash"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("completion must not be a valid command")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", stdout.String())
	}
}
