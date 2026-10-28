// Command hipoteca is the CLI entry point. It only wires the process to internal/cli:
// argument parsing, output and exit codes are decided there.
package main

import (
	"fmt"
	"os"

	"github.com/aam9063/hipoteca-cli/internal/cli"
)

// Injected at release time by GoReleaser with -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	if err := root.Execute(); err != nil {
		// stderr only: stdout belongs to the JSON result (P-01).
		fmt.Fprintln(os.Stderr, err)
		// CORE-01 replaces this with the stable exit codes of the arranque, section 6.4.
		os.Exit(1)
	}
}
