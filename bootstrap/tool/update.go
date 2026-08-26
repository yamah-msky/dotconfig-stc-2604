package main

// update: rewrite the pinned versions in the manifests, leaving the result as a
// reviewable git diff. Nothing here installs anything -- `bs.sh` does that, and
// only from what is committed. That separation is the whole point: versions move
// when you decide they move.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type manifestChange struct {
	label    string
	file     string
	key      string
	column   int
	from, to string
}

// Resolver variables keep Update deterministic under test without changing its
// production API or requiring network access.
var (
	resolveLatestRelease = LatestReleaseTag
	resolveLatestNode    = LatestNodeLTS
	resolveLatestPnpm    = LatestPnpm
	resolveLatestGo      = LatestGo
	resolveLatestNpm     = LatestNpmPackage
	resolveRemoteHead    = RemoteHead
)

func Update(e *Env, only []string, check bool) int {
	selected := func(name string) bool {
		if len(only) == 0 {
			return true
		}
		for _, o := range only {
			if strings.HasPrefix(name, o) {
				return true
			}
		}
		return false
	}

	headf("Resolving latest versions")

	// Everything that needs the network, gathered first so it can go in parallel.
	jobs := map[string]func() (string, error){}
	for _, t := range e.Tools {
		t := t
		if !selected(t.Name) || t.Floating() {
			continue
		}
		jobs["tool:"+t.Name] = func() (string, error) { return resolveLatestRelease(t.Repo) }
	}
	for _, g := range e.NpmGlobals {
		g := g
		if !selected(g.Name) || g.Floating() {
			continue
		}
		jobs["npm:"+g.Name] = func() (string, error) { return resolveLatestNpm(g.Package) }
	}
	if selected("nvim") {
		jobs["rt:nvim"] = func() (string, error) { return resolveLatestRelease("neovim/neovim") }
	}
	if selected("nvm") {
		jobs["rt:nvm"] = func() (string, error) { return resolveLatestRelease("nvm-sh/nvm") }
	}
	if selected("node") {
		jobs["rt:node"] = resolveLatestNode
	}
	if selected("pnpm") {
		jobs["rt:pnpm"] = resolveLatestPnpm
	}
	if selected("go") {
		jobs["rt:go"] = resolveLatestGo
	}
	for _, p := range e.Plugins {
		p := p
		if !selected("sheldon") && !selected(p.Short()) {
			continue
		}
		jobs["plugin:"+p.Repo] = func() (string, error) { return resolveRemoteHead(p.Repo) }
	}

	got := resolveAll(jobs)

	// Resolve and validate everything before writing anything. A partial network
	// failure must not produce a manifest assembled from different update runs.
	var changes []manifestChange
	var problems []string

	// tools.tsv, in manifest order rather than map order.
	for _, t := range e.Tools {
		if t.Floating() {
			if selected(t.Name) {
				skipf("%-12s %s (floats by design)", t.Name, t.Ref)
			}
			continue
		}
		r, ok := got["tool:"+t.Name]
		if !ok {
			continue
		}
		if r.err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", t.Name, r.err))
			continue
		}
		if r.latest == t.Ref {
			skipf("%-12s %s", t.Name, t.Ref)
			continue
		}
		changes = append(changes, manifestChange{t.Name, "tools.tsv", t.Name, 2, t.Ref, r.latest})
	}

	// npm-globals.tsv, in manifest order.
	for _, g := range e.NpmGlobals {
		if g.Floating() {
			if selected(g.Name) {
				skipf("%-12s %s (floats by design)", g.Name, g.Ref)
			}
			continue
		}
		r, ok := got["npm:"+g.Name]
		if !ok {
			continue
		}
		if r.err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", g.Name, r.err))
			continue
		}
		if r.latest == g.Ref {
			skipf("%-12s %s", g.Name, g.Ref)
			continue
		}
		changes = append(changes, manifestChange{g.Name, "npm-globals.tsv", g.Name, 3, g.Ref, r.latest})
	}

	// runtimes.tsv. rust tracks stable on purpose and is never bumped.
	for _, name := range []string{"nvim", "nvm", "node", "pnpm", "go"} {
		rt, ok := e.Runtimes[name]
		if !ok {
			continue
		}
		r, ok := got["rt:"+name]
		if !ok {
			continue
		}
		if r.err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, r.err))
			continue
		}
		if r.latest == rt.Ref {
			skipf("%-12s %s", name, rt.Ref)
			continue
		}
		changes = append(changes, manifestChange{name, "runtimes.tsv", name, 2, rt.Ref, r.latest})
	}
	if rt, ok := e.Runtimes["rust"]; ok && selected("rust") {
		skipf("%-12s %s (tracks upstream by design)", "rust", rt.Ref)
	}

	// Work out plugin edits before crossing the mutation boundary as well.
	pluginRevs := map[string]string{}
	for _, p := range e.Plugins {
		r, ok := got["plugin:"+p.Repo]
		if !ok {
			continue
		}
		if r.err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", p.Short(), r.err))
			continue
		}
		if r.latest == p.Rev {
			skipf("%-12s %s", p.Short(), short(p.Rev))
			continue
		}
		infof("%-12s %s -> %s", p.Short(), short(p.Rev), short(r.latest))
		pluginRevs[p.Repo] = r.latest
	}

	// Report proposed TSV edits.
	for _, c := range changes {
		infof("%-12s %s -> %s", c.label, c.from, c.to)
	}

	for _, p := range problems {
		warnf("%s", p)
	}
	if len(problems) > 0 {
		warnf("nothing was written because one or more versions could not be resolved")
		return 1
	}

	if !check {
		if err := applyManifestChanges(e, changes, pluginRevs); err != nil {
			errf("applying manifest updates: %v", err)
			return 1
		}
	}

	// nvim plugins: lazy.nvim owns lazy-lock.json, so ask it rather than editing.
	nvimChanged := false
	if len(only) == 0 || selected("plugins") || selected("nvim") {
		switch {
		case Which("nvim") == "":
			skipf("nvim plugins (neovim is not installed)")
		case check:
			skipf("nvim plugins (run without --check to update lazy-lock.json)")
		default:
			infof("updating nvim plugins via lazy.nvim")
			if !commandRunsTimeout(10*time.Minute, "nvim", "--headless", "+Lazy! update", "+qa") {
				problems = append(problems, "nvim plugin update failed or timed out")
			}
			if !commandRuns("git", "-C", e.ConfigDir, "diff", "--quiet", "--", "nvim/lazy-lock.json") {
				nvimChanged = true
				infof("lazy-lock.json updated")
			} else {
				skipf("lazy-lock.json unchanged")
			}
		}
	}
	for _, p := range problems {
		warnf("%s", p)
	}

	total := len(changes) + len(pluginRevs)
	fmt.Println()
	if total == 0 && !nvimChanged {
		okf("everything is already at the latest version")
		if len(problems) > 0 {
			return 1
		}
		return 0
	}
	if check {
		warnf("updates are available (run without --check to write them)")
		return 1
	}

	headf("Updated")
	fmt.Printf(`  Review and commit, then apply:

      git -C %s diff
      git -C %s commit -am 'Bump pins'
      %s
	`, e.ConfigDir, e.ConfigDir, filepath.Join(e.BootstrapDir, "bs.sh"))
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// setTSVField rewrites one cell of the row whose first field is key, preserving
// comments, blank lines, spacing and -- critically -- the column count. A
// whole-file substitution would be far shorter and would silently corrupt a
// column the moment a version string appeared anywhere else.
func renderTSVChanges(path string, content []byte, changes []manifestChange) (string, error) {
	var out []string
	found := make(map[string]bool, len(changes))
	wantFields := -1

	sc := bufio.NewScanner(strings.NewReader(string(content)))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		fields := strings.Split(line, "\t")
		if wantFields == -1 {
			wantFields = len(fields)
		} else if len(fields) != wantFields {
			return "", fmt.Errorf("%s: row %q has %d fields, expected %d",
				filepath.Base(path), fields[0], len(fields), wantFields)
		}
		for _, c := range changes {
			if fields[0] != c.key {
				continue
			}
			if c.column < 1 || c.column > len(fields) {
				return "", fmt.Errorf("column %d out of range for row %q", c.column, c.key)
			}
			fields[c.column-1] = c.to
			found[c.key] = true
		}
		out = append(out, strings.Join(fields, "\t"))
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	for _, c := range changes {
		if !found[c.key] {
			return "", fmt.Errorf("no row named %q in %s", c.key, filepath.Base(path))
		}
	}
	return strings.Join(out, "\n") + "\n", nil
}

// setPluginRev replaces the rev of one [plugins.*] block. Matching is positional
// -- the rev belongs to the most recent github line -- because that is how the
// file is written and it avoids pulling in a TOML parser.
func renderPluginRevs(content []byte, revisions map[string]string) (string, error) {
	lines := strings.Split(string(content), "\n")
	current := ""
	replaced := map[string]bool{}
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "github = "):
			current = unquote(strings.TrimPrefix(line, "github = "))
		case strings.HasPrefix(line, "rev = "):
			if rev, ok := revisions[current]; ok {
				lines[i] = fmt.Sprintf("rev = %q", rev)
				replaced[current] = true
			}
		}
	}
	for repo := range revisions {
		if !replaced[repo] {
			return "", fmt.Errorf("no rev line found for %s", repo)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// applyManifestChanges pre-renders every affected file before the first write,
// then writes each file once. This catches malformed rows without leaving a
// subset of the requested changes behind.
func applyManifestChanges(e *Env, changes []manifestChange, pluginRevs map[string]string) error {
	byFile := map[string][]manifestChange{}
	for _, c := range changes {
		byFile[c.file] = append(byFile[c.file], c)
	}
	rendered := map[string]string{}
	for file, cs := range byFile {
		path := filepath.Join(e.BootstrapDir, file)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content, err := renderTSVChanges(path, b, cs)
		if err != nil {
			return err
		}
		rendered[path] = content
	}
	if len(pluginRevs) > 0 {
		path := filepath.Join(e.ConfigDir, "sheldon", "plugins.toml")
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content, err := renderPluginRevs(b, pluginRevs)
		if err != nil {
			return err
		}
		rendered[path] = content
	}
	for path, content := range rendered {
		if err := writeAtomic(path, content); err != nil {
			return err
		}
	}
	return nil
}

func writeAtomic(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bstool-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename succeeds

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Keep the original mode rather than the 0600 CreateTemp gives us.
	if fi, err := os.Stat(path); err == nil {
		_ = os.Chmod(name, fi.Mode())
	}
	return os.Rename(name, path)
}
