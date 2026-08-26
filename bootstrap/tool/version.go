package main

// Version extraction and comparison.
//
// This is where the shell version had its real bugs, so it is the part that
// carries tests. Two of them are worth naming:
//
//   - comparing versions as strings puts 0.10.0 *below* 0.9.3, which is how an
//     old distro zoxide passed for a newer pin.
//   - several tools do not print their version on the first line, or print
//     somebody else's version on the second.

import (
	"regexp"
	"strconv"
	"strings"
)

var verRE = regexp.MustCompile(`[0-9]+(\.[0-9]+){1,2}`)

// ExtractVersion pulls the first version-shaped token out of arbitrary
// --version output. Verified quirks, each of which has a test:
//
//	eza      prints its version on line 2; line 1 contains no digits at all
//	sheldon  prints its own version on line 1 and rustc's on line 2
//	go       "go version go1.22.2 linux/amd64"
//	node     "v24.15.0"
//	fzf      "0.74.1 (eae8d9d2)"
//	yt-dlp   "2026.03.17" -- a date, but it still orders correctly as a version
//
// Taking the *first* match across the whole output handles all of them: the
// tools whose own version is not on line 1 have no digits before it.
func ExtractVersion(out string) string {
	// "go version go1.22.2 ..." would otherwise yield nothing useful, since the
	// number is glued to "go".
	out = strings.ReplaceAll(out, "go1.", "1.")
	return verRE.FindString(out)
}

// InstalledVersion runs a tool and reports its version, or "" if it cannot be
// determined. A non-zero exit is not fatal: some tools print their version and
// exit 1.
func InstalledVersion(bin string) string {
	var args []string
	switch bin {
	case "go":
		args = []string{"version"}
	case "node":
		args = []string{"-v"}
	default:
		args = []string{"--version"}
	}
	return ExtractVersion(commandCombinedOutput(bin, args...))
}

// CompareVersions returns -1, 0 or +1. Components are compared numerically, so
// 0.10.0 > 0.9.3. A shorter version is padded with zeros, so 1.2 == 1.2.0.
// Non-numeric junk sorts below anything numeric.
func CompareVersions(a, b string) int {
	as, bs := splitVer(a), splitVer(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		x, y := at(as, i), at(bs, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// AtLeast reports a >= b. Empty or unparseable input is never "at least".
func AtLeast(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return CompareVersions(a, b) >= 0
}

func splitVer(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		// Take each component's leading digits, so "1.2.3-rc1" compares as 1.2.3
		// rather than collapsing to 1.2. A component with no leading digit ends
		// the version. We never compare pre-releases against each other -- pins
		// are exact tags -- so ignoring the suffix is fine and predictable.
		digits := p
		for i, r := range p {
			if r < '0' || r > '9' {
				digits = p[:i]
				break
			}
		}
		if digits == "" {
			break
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

func at(s []int, i int) int {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// NeedsInstall reports whether work is required for a pinned or floating ref.
// Mirrors needs_install() in lib.sh: an exact pin is compared for equality, not
// for "at least", because a version newer than the pin is drift too.
func NeedsInstall(current, ref, min string) bool {
	if current == "" {
		return true
	}
	if ref != refLatest {
		return current != strings.TrimPrefix(ref, "v")
	}
	if min == "-" || min == "" {
		return false
	}
	return !AtLeast(current, min)
}
