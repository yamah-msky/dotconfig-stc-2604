package main

// update: rewrite the pinned versions in the manifests, leaving the result as a
// reviewable git diff. Nothing here installs anything -- `bs.sh` does that, and
// only from what is committed. That separation is the whole point: versions move
// when you decide they move.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		jobs["tool:"+t.Name] = func() (string, error) { return LatestReleaseTag(t.Repo) }
	}
	for _, g := range e.NpmGlobals {
		g := g
		if !selected(g.Name) || g.Floating() {
			continue
		}
		jobs["npm:"+g.Name] = func() (string, error) { return LatestNpmPackage(g.Package) }
	}
	if selected("nvim") {
		jobs["rt:nvim"] = func() (string, error) { return LatestReleaseTag("neovim/neovim") }
	}
	if selected("node") {
		jobs["rt:node"] = LatestNodeLTS
	}
	if selected("pnpm") {
		jobs["rt:pnpm"] = LatestPnpm
	}
	if selected("go") {
		jobs["rt:go"] = LatestGo
	}
	for _, p := range e.Plugins {
		p := p
		if !selected("sheldon") && !selected(p.Short()) {
			continue
		}
		jobs["plugin:"+p.Repo] = func() (string, error) { return RemoteHead(p.Repo) }
	}

	got := resolveAll(jobs)

	// changes are applied after every lookup is in, so a partial network failure
	// cannot leave the manifests half-bumped.
	type change struct {
		label    string
		file     string
		key      string
		column   int
		from, to string
	}
	var changes []change
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
		changes = append(changes, change{t.Name, "tools.tsv", t.Name, 2, t.Ref, r.latest})
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
		changes = append(changes, change{g.Name, "npm-globals.tsv", g.Name, 3, g.Ref, r.latest})
	}

	// runtimes.tsv. rust tracks stable on purpose and is never bumped.
	for _, name := range []string{"nvim", "node", "pnpm", "go"} {
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
		changes = append(changes, change{name, "runtimes.tsv", name, 2, rt.Ref, r.latest})
	}
	if rt, ok := e.Runtimes["rust"]; ok && selected("rust") {
		skipf("%-12s %s (tracks upstream by design)", "rust", rt.Ref)
	}

	// Report and apply the TSV edits.
	for _, c := range changes {
		infof("%-12s %s -> %s", c.label, c.from, c.to)
	}
	if !check {
		for _, c := range changes {
			if err := setTSVField(filepath.Join(e.BootstrapDir, c.file), c.key, c.column, c.to); err != nil {
				errf("%s: %v", c.file, err)
				return 1
			}
		}
	}

	// sheldon plugin revisions live in plugins.toml, not a TSV.
	pluginChanges := 0
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
		pluginChanges++
		if !check {
			if err := setPluginRev(e, p.Repo, r.latest); err != nil {
				errf("plugins.toml: %v", err)
				return 1
			}
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
			cmd := exec.Command("nvim", "--headless", "+Lazy! update", "+qa")
			if err := cmd.Run(); err != nil {
				problems = append(problems, fmt.Sprintf("nvim plugin update: %v", err))
			}
			lock := exec.Command("git", "-C", e.ConfigDir, "diff", "--quiet", "--", "nvim/lazy-lock.json")
			if lock.Run() != nil {
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

	total := len(changes) + pluginChanges
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
func setTSVField(path, key string, column int, value string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var out []string
	found := false
	wantFields := -1

	sc := bufio.NewScanner(f)
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
			f.Close()
			return fmt.Errorf("%s: row %q has %d fields, expected %d",
				filepath.Base(path), fields[0], len(fields), wantFields)
		}
		if fields[0] == key {
			if column < 1 || column > len(fields) {
				f.Close()
				return fmt.Errorf("column %d out of range for row %q", column, key)
			}
			fields[column-1] = value
			found = true
		}
		out = append(out, strings.Join(fields, "\t"))
	}
	if err := sc.Err(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if !found {
		return fmt.Errorf("no row named %q in %s", key, filepath.Base(path))
	}

	// Write via a temporary file in the same directory, then rename, so an
	// interrupted update cannot leave a truncated manifest behind.
	return writeAtomic(path, strings.Join(out, "\n")+"\n")
}

// setPluginRev replaces the rev of one [plugins.*] block. Matching is positional
// -- the rev belongs to the most recent github line -- because that is how the
// file is written and it avoids pulling in a TOML parser.
func setPluginRev(e *Env, repo, newRev string) error {
	path := filepath.Join(e.ConfigDir, "sheldon", "plugins.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	current := ""
	replaced := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "github = "):
			current = unquote(strings.TrimPrefix(line, "github = "))
		case strings.HasPrefix(line, "rev = ") && current == repo:
			lines[i] = fmt.Sprintf("rev = %q", newRev)
			replaced = true
		}
	}
	if !replaced {
		return fmt.Errorf("no rev line found for %s", repo)
	}
	return writeAtomic(path, strings.Join(lines, "\n"))
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
