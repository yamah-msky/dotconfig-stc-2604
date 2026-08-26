package main

// Where everything lives, resolved once. Mirrors the constants at the top of
// bootstrap/lib.sh; BOOTSTRAP_DIR and CONFIG_DIR are passed in by bs.sh so there
// is one authority for them.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Env struct {
	BootstrapDir string
	ConfigDir    string
	Home         string
	XDGData      string
	XDGCache     string
	LocalBin     string
	FontDir      string
	GoRoot       string
	NvmDir       string
	PnpmHome     string
	Arch         Arch

	Tools      []Tool
	Runtimes   map[string]Runtime
	AptGroups  []AptGroup
	Plugins    []Plugin
	NpmGlobals []NpmGlobal
}

func NewEnv() (*Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	e := &Env{Home: home}

	e.BootstrapDir = os.Getenv("BOOTSTRAP_DIR")
	if e.BootstrapDir == "" {
		// Standalone invocation: assume we sit in bootstrap/tool.
		exe, err := os.Executable()
		if err == nil {
			if d := filepath.Dir(filepath.Dir(exe)); fileExists(filepath.Join(d, "tools.tsv")) {
				e.BootstrapDir = d
			}
		}
		if e.BootstrapDir == "" {
			e.BootstrapDir = filepath.Join(home, ".config", "bootstrap")
		}
	}
	e.ConfigDir = os.Getenv("CONFIG_DIR")
	if e.ConfigDir == "" {
		e.ConfigDir = filepath.Dir(e.BootstrapDir)
	}

	e.XDGData = envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	e.XDGCache = envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	e.LocalBin = filepath.Join(home, ".local", "bin")
	e.FontDir = filepath.Join(e.XDGData, "fonts")
	e.GoRoot = filepath.Join(home, ".local", "go")
	e.NvmDir = envOr("NVM_DIR", filepath.Join(e.ConfigDir, "nvm"))
	e.PnpmHome = envOr("PNPM_HOME", filepath.Join(e.XDGData, "pnpm"))

	uname := runtime.GOARCH
	if out, err := exec.Command("uname", "-m").Output(); err == nil {
		uname = strings.TrimSpace(string(out))
	}
	if o := os.Getenv("ARCH_OVERRIDE"); o != "" {
		uname = o
	}
	e.Arch = ArchFor(uname)

	if e.Tools, err = LoadTools(e.BootstrapDir); err != nil {
		return nil, fmt.Errorf("loading tools.tsv: %w", err)
	}
	rts, err := LoadRuntimes(e.BootstrapDir)
	if err != nil {
		return nil, fmt.Errorf("loading runtimes.tsv: %w", err)
	}
	e.Runtimes = map[string]Runtime{}
	for _, r := range rts {
		e.Runtimes[r.Name] = r
	}
	if e.AptGroups, err = LoadAptGroups(e.BootstrapDir); err != nil {
		return nil, fmt.Errorf("loading apt.tsv: %w", err)
	}
	// Not fatal: a machine could legitimately have no sheldon config yet.
	e.Plugins, _ = LoadPlugins(e.ConfigDir)
	if e.NpmGlobals, err = LoadNpmGlobals(e.BootstrapDir); err != nil {
		return nil, fmt.Errorf("loading npm-globals.tsv: %w", err)
	}

	return e, nil
}

// Tilde shortens a path for display, so the table stays narrow.
func (e *Env) Tilde(p string) string {
	if strings.HasPrefix(p, e.Home+"/") {
		return "~" + strings.TrimPrefix(p, e.Home)
	}
	return p
}

// Which is exec.LookPath with the error dropped, since every caller only wants
// the path or "".
func Which(bin string) string {
	p, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	return p
}

// Runs reports whether a tool on PATH actually works, which Which cannot tell:
// a wrapper script whose native binary was never fetched is an ordinary
// executable file that fails on every call. Only for tools whose --version is
// cheap and side-effect free.
func Runs(bin string) bool {
	return commandRuns(bin, "--version")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isWSL() bool {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// output runs a command and returns its combined output, trimmed. Errors are
// folded into the empty string: every caller here treats "could not determine"
// and "failed" the same way.
var localCommandTimeout = 8 * time.Second

func commandOutputTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// A killed shell can leave a grandchild holding the output pipe open. Do not
	// let that defeat the context deadline while os/exec waits for EOF.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s timed out after %s", name, timeout)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commandOutput(name string, args ...string) (string, error) {
	return commandOutputTimeout(localCommandTimeout, name, args...)
}

func commandCombinedOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), localCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out))
}

func commandRuns(name string, args ...string) bool {
	return commandRunsTimeout(localCommandTimeout, name, args...)
}

func commandRunsTimeout(timeout time.Duration, name string, args ...string) bool {
	_, err := commandOutputTimeout(timeout, name, args...)
	return err == nil
}

func output(name string, args ...string) string {
	out, _ := commandOutput(name, args...)
	return out
}
