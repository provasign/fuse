package cli

import (
	"os"
	"os/exec"
)

// newCmd is a small wrapper to ease testing of git invocation.
func newCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}
