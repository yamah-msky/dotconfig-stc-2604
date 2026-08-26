package main

// bstool -- the doctor and update halves of the bootstrap.
//
// Invoked through `bs.sh doctor` and `bs.sh update`, which build and cache this
// binary on first use. Not part of installing a machine: `bs.sh` alone brings a
// bare Ubuntu up, including the Go this is built with. That ordering is why the
// install path stays in shell and only these two commands live here -- they are
// the parts with real logic (HTTP, JSON, version algebra) and they are never
// needed before Go exists.

import (
	"fmt"
	"os"
	"strings"
)

const usage = `Usage: bstool <doctor|update> [options]

  doctor            report where this machine differs from the manifests
  update [name...]  bump pinned versions; with no names, everything

Options
  --check           update only: check manifest pins; lazy-lock is not queried
  --json            doctor only: machine-readable output
  -h, --help
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(2)
	}

	cmd := args[0]
	args = args[1:]
	var names []string
	check, asJSON := false, false
	for _, a := range args {
		switch a {
		case "--check":
			check = true
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Print(usage)
			return
		default:
			if strings.HasPrefix(a, "-") {
				errf("unknown option: %s", a)
				os.Exit(2)
			}
			names = append(names, a)
		}
	}

	env, err := NewEnv()
	if err != nil {
		errf("%v", err)
		os.Exit(1)
	}

	switch cmd {
	case "doctor":
		os.Exit(Doctor(env, asJSON))
	case "update":
		os.Exit(Update(env, names, check))
	case "-h", "--help":
		fmt.Print(usage)
	default:
		errf("unknown command: %s", cmd)
		fmt.Print(usage)
		os.Exit(2)
	}
}

// ----------------------------------------------------------------------------
// Output, matching lib.sh's prefixes so the two halves look like one tool
// ----------------------------------------------------------------------------

var color = os.Getenv("NO_COLOR") == "" && isTTY()

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func colorFor(s Status) string {
	if !color {
		return ""
	}
	switch s {
	case StatusOK:
		return "\033[1;32m"
	case StatusWarn:
		return "\033[1;33m"
	case StatusFail:
		return "\033[1;31m"
	default:
		return "\033[2m"
	}
}

func colorOff() string {
	if !color {
		return ""
	}
	return "\033[0m"
}

func wrap(c, s string) string {
	if !color {
		return s
	}
	return c + s + "\033[0m"
}

func headf(format string, a ...any) {
	fmt.Printf("\n%s %s\n", wrap("\033[1;36m", "==>"), fmt.Sprintf(format, a...))
}
func infof(format string, a ...any) {
	fmt.Printf("%s %s\n", wrap("\033[1;34m", "  ->"), fmt.Sprintf(format, a...))
}
func okf(format string, a ...any) {
	fmt.Printf("%s %s\n", wrap("\033[1;32m", "  OK"), fmt.Sprintf(format, a...))
}
func skipf(format string, a ...any) {
	fmt.Printf("%s %s\n", wrap("\033[2m", "  --"), fmt.Sprintf(format, a...))
}
func warnf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", wrap("\033[1;33m", "  !!"), fmt.Sprintf(format, a...))
}
func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", wrap("\033[1;31m", "  XX"), fmt.Sprintf(format, a...))
}
