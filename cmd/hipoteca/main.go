// Command hipoteca is the CLI entry point. It only wires the process to internal/cli:
// argument parsing, output and exit codes are decided there.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/aam9063/hipoteca-cli/internal/cli"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/output"
)

// Injected at release time by GoReleaser with -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Ctrl+C interrupts the process: exit 130 without an error envelope (§6.4).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := cli.NewRootCmd(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	err := root.ExecuteContext(ctx)

	code, env := exitCodeFor(ctx, err)
	if env != nil {
		writeEnvelope(*env)
	}
	if code != 0 {
		os.Exit(code)
	}
}

// exitCodeFor decides the process outcome (§6.4): an interruption outranks
// whatever error the command returned and yields 130 without an envelope; a
// contract error yields its envelope and its stable exit code; success yields
// 0 with no envelope. Extracted from main so the decision is unit-testable.
func exitCodeFor(ctx context.Context, err error) (int, *errs.Envelope) {
	if ctx.Err() != nil {
		return errs.CodeInterrupted, nil
	}
	if err == nil {
		return 0, nil
	}
	env, ok := errs.From(err)
	if !ok {
		// An error that does not carry a contract kind is unexpected: it is
		// reported as internal (code 1) and never with an invented code.
		env, _ = errs.From(errs.New(errs.KindInternal, err.Error(), ""))
	}
	return env.Error.ExitCode, &env
}

// writeEnvelope sends the error to stderr only: stdout belongs to the JSON
// result and must stay empty on failure (P-01). JSON when stdout is not a
// terminal (an agent always receives JSON), plain text when it is (§6.1).
func writeEnvelope(env errs.Envelope) {
	if isTerminal(os.Stdout) {
		fmt.Fprintf(os.Stderr, "Error: %s\n", env.Error.Message)
		return
	}
	_ = output.WriteJSON(os.Stderr, env)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
