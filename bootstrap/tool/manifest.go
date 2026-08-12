package main

// Reading the three TSV manifests into types. This is the main reason the tool
// exists in Go rather than shell: the shell version addressed columns by number
// (`cut -f2`), which is exactly the kind of thing that silently reads the wrong
// field when a column is added.
//
// The parsing rules here must match bootstrap/lib.sh, which the install path
// still uses: same placeholder set, same "tag verbatim" convention, same URL
// shape. Where a rule is subtle the comment says why.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Arch holds every naming scheme upstreams use for the same machine. Mirrors
// arch_init() in lib.sh.
type Arch struct {
	Raw      string
	Bare     string // x86_64          -- {ARCH}
	RustGNU  string // x86_64-unknown-linux-gnu
	RustMusl string // x86_64-unknown-linux-musl
	GoArch   string // amd64           -- Go-style
	DebArch  string // amd64
	BobArch  string // x86_64          -- bob calls arm64 "arm"
}

func ArchFor(uname string) Arch {
	switch uname {
	case "x86_64", "amd64":
		return Arch{uname, "x86_64",
			"x86_64-unknown-linux-gnu", "x86_64-unknown-linux-musl",
			"amd64", "amd64", "x86_64"}
	case "aarch64", "arm64":
		return Arch{uname, "aarch64",
			"aarch64-unknown-linux-gnu", "aarch64-unknown-linux-musl",
			"arm64", "arm64", "arm"}
	}
	return Arch{Raw: uname}
}

// Supported reports whether prebuilt assets exist for this machine at all.
func (a Arch) Supported() bool { return a.RustGNU != "" }

// Tool is one row of tools.tsv.
type Tool struct {
	Name   string
	Ref    string // the upstream release tag, verbatim
	Min    string // lowest version our configs accept, or "-"
	Repo   string // owner/name
	Asset  string // filename template
	Bins   []string
	IsFont bool
}

// Floating rows resolve at install time and are never bumped by update.
func (t Tool) Floating() bool { return t.Ref == refLatest }

const refLatest = "latest"

// Version is Ref without a leading v. Upstreams disagree about the prefix --
// sheldon and uv publish "0.8.5", everyone else "v0.8.5" -- so the manifest
// stores the tag as published and this derives the comparable number.
func (t Tool) Version() string { return strings.TrimPrefix(t.Ref, "v") }

// Probe is the binary whose --version stands for the whole row.
func (t Tool) Probe() string {
	if len(t.Bins) == 0 {
		return t.Name
	}
	return t.Bins[0]
}

func (t Tool) AssetName(a Arch) string {
	r := strings.NewReplacer(
		"{TAG}", t.Ref,
		"{VERSION}", t.Version(),
		"{ARCH}", a.Bare,
		"{RUST_GNU}", a.RustGNU,
		"{RUST_MUSL}", a.RustMusl,
		"{GOARCH}", a.GoArch,
		"{DEB_ARCH}", a.DebArch,
		"{BOB_ARCH}", a.BobArch,
	)
	return r.Replace(t.Asset)
}

func (t Tool) DownloadURL(a Arch) string {
	asset := t.AssetName(a)
	if t.Floating() {
		return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", t.Repo, asset)
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", t.Repo, t.Ref, asset)
}

// NpmGlobal is one row of npm-globals.tsv: a user-declared CLI installed via
// `pnpm add -g` into $PNPM_HOME, kept independent of whichever node version is
// currently the nvm default. See the file's header comment for why that
// independence matters.
type NpmGlobal struct {
	Name    string
	Package string
	Ref     string // pinned version, or "latest" to float
	Bin     string // binary name, when it differs from Name
}

// Floating rows are never bumped by update, same convention as Tool.
func (g NpmGlobal) Floating() bool { return g.Ref == refLatest }

// Probe is the binary whose --version stands for the row.
func (g NpmGlobal) Probe() string {
	if g.Bin != "" {
		return g.Bin
	}
	return g.Name
}

// Runtime is one row of runtimes.tsv.
type Runtime struct {
	Name, Ref, Min, How string
}

// Tracking reports a ref that follows upstream rather than pinning it (rust).
func (r Runtime) Tracking() bool { return r.Ref == "stable" }

// AptGroup collects every packages cell sharing a group name; a group may span
// several lines in apt.tsv.
type AptGroup struct {
	Name     string
	Packages []string
}

// Plugin is a [plugins.*] entry of sheldon/plugins.toml. Parsed by hand rather
// than with a TOML library, to keep the dependency count at zero.
type Plugin struct {
	Repo string // owner/name
	Rev  string // pinned commit, "" when unpinned
}

func (p Plugin) Short() string {
	if i := strings.LastIndex(p.Repo, "/"); i >= 0 {
		return p.Repo[i+1:]
	}
	return p.Repo
}

// ----------------------------------------------------------------------------
// Loading
// ----------------------------------------------------------------------------

// rows returns the non-comment, non-blank lines of a TSV, split on tabs.
func rows(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		out = append(out, strings.Split(line, "\t"))
	}
	return out, sc.Err()
}

func LoadTools(dir string) ([]Tool, error) {
	rs, err := rows(filepath.Join(dir, "tools.tsv"))
	if err != nil {
		return nil, err
	}
	var out []Tool
	for i, r := range rs {
		if len(r) != 6 {
			return nil, fmt.Errorf("tools.tsv row %d (%q): want 6 tab-separated fields, got %d",
				i+1, r[0], len(r))
		}
		t := Tool{Name: r[0], Ref: r[1], Min: r[2], Repo: r[3], Asset: r[4]}
		switch install := r[5]; {
		case install == "font":
			t.IsFont = true
		case strings.HasPrefix(install, "bin:"):
			t.Bins = strings.Split(strings.TrimPrefix(install, "bin:"), ",")
		default:
			return nil, fmt.Errorf("tools.tsv row %d (%s): install must be `font` or `bin:...`, got %q",
				i+1, t.Name, install)
		}
		out = append(out, t)
	}
	return out, nil
}

func LoadRuntimes(dir string) ([]Runtime, error) {
	rs, err := rows(filepath.Join(dir, "runtimes.tsv"))
	if err != nil {
		return nil, err
	}
	var out []Runtime
	for i, r := range rs {
		if len(r) != 4 {
			return nil, fmt.Errorf("runtimes.tsv row %d (%q): want 4 fields, got %d", i+1, r[0], len(r))
		}
		out = append(out, Runtime{r[0], r[1], r[2], r[3]})
	}
	return out, nil
}

func LoadNpmGlobals(dir string) ([]NpmGlobal, error) {
	rs, err := rows(filepath.Join(dir, "npm-globals.tsv"))
	if err != nil {
		return nil, err
	}
	var out []NpmGlobal
	for i, r := range rs {
		if len(r) != 4 {
			return nil, fmt.Errorf("npm-globals.tsv row %d (%q): want 4 tab-separated fields, got %d",
				i+1, r[0], len(r))
		}
		out = append(out, NpmGlobal{Name: r[0], Package: r[1], Ref: r[2], Bin: r[3]})
	}
	return out, nil
}

func LoadAptGroups(dir string) ([]AptGroup, error) {
	rs, err := rows(filepath.Join(dir, "apt.tsv"))
	if err != nil {
		return nil, err
	}
	// Preserve first-seen order; a group legitimately appears on several lines.
	var order []string
	byName := map[string][]string{}
	for i, r := range rs {
		if len(r) != 2 {
			return nil, fmt.Errorf("apt.tsv row %d (%q): want 2 fields, got %d", i+1, r[0], len(r))
		}
		if _, seen := byName[r[0]]; !seen {
			order = append(order, r[0])
		}
		byName[r[0]] = append(byName[r[0]], strings.Fields(r[1])...)
	}
	out := make([]AptGroup, 0, len(order))
	for _, n := range order {
		out = append(out, AptGroup{n, byName[n]})
	}
	return out, nil
}

// LoadPlugins reads sheldon/plugins.toml. Only lines starting at column zero
// count: the file ships a commented-out example, and matching it would report a
// plugin that does not exist.
func LoadPlugins(configDir string) ([]Plugin, error) {
	f, err := os.Open(filepath.Join(configDir, "sheldon", "plugins.toml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Plugin
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "github = "):
			out = append(out, Plugin{Repo: unquote(strings.TrimPrefix(line, "github = "))})
		case strings.HasPrefix(line, "rev = ") && len(out) > 0:
			out[len(out)-1].Rev = unquote(strings.TrimPrefix(line, "rev = "))
		}
	}
	return out, sc.Err()
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return s
}
