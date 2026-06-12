package cli

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// newCmd is a small wrapper to ease testing of git invocation.
func newCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// newShellCmd runs a user-provided command line through the platform shell,
// so agent commands like "claude -p" or pipelines work as typed.
//
// Unix: $SHELL (fallback /bin/sh) -c. Windows: %COMSPEC% /C (cmd.exe), or
// -Command when COMSPEC points at PowerShell. FUSE_SHELL overrides the
// shell on every platform.
func newShellCmd(cmdline string) *exec.Cmd {
	if custom := os.Getenv("FUSE_SHELL"); custom != "" {
		return exec.Command(custom, shellFlag(custom), cmdline)
	}
	if runtime.GOOS == "windows" {
		comspec := os.Getenv("COMSPEC")
		if comspec == "" {
			comspec = "cmd.exe"
		}
		return exec.Command(comspec, shellFlag(comspec), cmdline)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return exec.Command(shell, "-c", cmdline)
}

// shellFlag returns the "run this command line" flag for a shell binary.
func shellFlag(shell string) string {
	name := strings.ToLower(shell)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "cmd":
		return "/C"
	case "powershell", "pwsh":
		return "-Command"
	default:
		return "-c"
	}
}
