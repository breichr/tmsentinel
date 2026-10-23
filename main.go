// Command tmsentinel is a stateless, FDA-free Time Machine monitoring
// CLI. It answers: is my Time Machine backup really running — and if
// not, why not? Read-only, no daemon, no GUI, no state file; every run
// derives state fresh from the Unified Log.
package main

import (
	"os"
	"time"

	"github.com/breichr/tmsentinel/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}
