// Package awg manages a RouteBox-owned AmneziaWG (awg-quick) server interface.
package awg

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner executes external commands. Production uses execRunner; tests inject a
// fake. ALWAYS arg-vector form (exec.Command) — never `sh -c`/`bash -c`, so no
// user string is ever interpreted by a shell.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

type execRunner struct{}

// NewExecRunner returns the production Runner.
func NewExecRunner() Runner { return execRunner{} }

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}
