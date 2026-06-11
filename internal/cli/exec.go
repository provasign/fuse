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

// newShellCmd runs a user-provided command line through the shell, so agent
// commands like "claude -p" or pipelines work as typed.
func newShellCmd(cmdline string) *exec.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return exec.Command(shell, "-c", cmdline)
}
