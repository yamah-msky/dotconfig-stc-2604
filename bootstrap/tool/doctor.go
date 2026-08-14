package main

// doctor: report where the machine differs from the manifests. Reads only.
//
// Ported from the check_* functions that used to live beside each install step
// in bootstrap/steps/*.sh. The manifest-driven rows are generated in a loop, so
// adding a line to tools.tsv still adds a doctor row for free. The bespoke ones
// carry their reasoning in comments, because most of them exist for a failure
// that is silent otherwise.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Status string

const (
	StatusOK   Status = "OK"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
	StatusInfo Status = "INFO"
)

type Result struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Bad reports whether a result should make doctor exit non-zero. INFO never does.
func (r Result) Bad() bool { return r.Status == StatusWarn || r.Status == StatusFail }

type report struct{ results []Result }

func (rp *report) add(id string, s Status, format string, args ...any) {
	rp.results = append(rp.results, Result{id, s, fmt.Sprintf(format, args...)})
}

func Doctor(e *Env, asJSON bool) int {
	rp := &report{}

	checkApt(e, rp)
	checkShims(e, rp)
	checkGh(e, rp)
	checkDocker(e, rp)
	checkLocale(e, rp)
	checkZdotdir(e, rp)
	checkTools(e, rp)
	checkRust(e, rp)
	checkGo(e, rp)
	checkNode(e, rp)
	checkPnpm(e, rp)
	checkNpmGlobals(e, rp)
	checkNvim(e, rp)
	checkGit(e, rp)
	checkZshPlugins(e, rp)
	checkShell(e, rp)
	checkPathContract(e, rp)
	checkWSL(e, rp)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rp.results)
	} else {
		printTable(rp.results)
	}

	for _, r := range rp.results {
		if r.Bad() {
			if !asJSON {
				fmt.Println()
				warnf("differences found -- run %s to reconcile", filepath.Join(e.BootstrapDir, "bs.sh"))
			}
			return 1
		}
	}
	if !asJSON {
		fmt.Println()
		okf("machine matches the manifests")
	}
	return 0
}

func printTable(rs []Result) {
	width := 3
	for _, r := range rs {
		if len(r.ID) > width {
			width = len(r.ID)
		}
	}
	fmt.Printf("%-*s %-6s %s\n", width, "ID", "STATUS", "DETAIL")
	fmt.Printf("%-*s %-6s %s\n", width, strings.Repeat("-", 3), "------", "------")
	for _, r := range rs {
		fmt.Printf("%-*s %s%-6s%s %s\n", width, r.ID, colorFor(r.Status), r.Status, colorOff(), r.Detail)
	}
}

// ----------------------------------------------------------------------------
// apt
// ----------------------------------------------------------------------------

// dpkgInstalled reports which of the given packages dpkg considers installed.
// One invocation for the lot: dpkg-query exits non-zero when any name is unknown
// but still prints the ones it found, so the error is ignored on purpose.
func dpkgInstalled(pkgs []string) map[string]bool {
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"-W", "-f=${Package} ${Status}\n"}, pkgs...)
	out, _ := exec.Command("dpkg-query", args...).Output()
	got := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && f[len(f)-1] == "installed" {
			got[f[0]] = true
		}
	}
	return got
}

func checkApt(e *Env, rp *report) {
	for _, g := range e.AptGroups {
		have := dpkgInstalled(g.Packages)
		var missing []string
		for _, p := range g.Packages {
			if !have[p] {
				missing = append(missing, p)
			}
		}
		id := "apt:" + g.Name
		if len(missing) == 0 {
			rp.add(id, StatusOK, "%d packages installed", len(g.Packages))
		} else {
			rp.add(id, StatusWarn, "%d of %d missing:%s",
				len(missing), len(g.Packages), " "+strings.Join(missing, " "))
		}
	}
}

// Ubuntu ships fd as fdfind and bat as batcat. zsh/aliases.zsh and the zsh-bat
// plugin call the short names, and fall back silently when they are absent --
// which is why this is worth a row of its own.
func checkShims(e *Env, rp *report) {
	for _, bin := range []string{"fd", "bat"} {
		if p := Which(bin); p != "" {
			rp.add("shim:"+bin, StatusOK, "%s", e.Tilde(p))
		} else {
			rp.add("shim:"+bin, StatusWarn, "missing -- zsh/aliases.zsh falls back to plain ls/cat")
		}
	}
}

func checkGh(e *Env, rp *report) {
	if Which("gh") == "" {
		rp.add("gh", StatusWarn, "not installed")
		return
	}
	rp.add("gh", StatusOK, "%s", InstalledVersion("gh"))
}

// ----------------------------------------------------------------------------
// Docker Engine
// ----------------------------------------------------------------------------

var dockerPackages = []string{
	"docker-ce", "docker-ce-cli", "containerd.io",
	"docker-buildx-plugin", "docker-compose-plugin",
}

func dockerRepoState() (repo, key bool) {
	paths := []string{"/etc/apt/sources.list"}
	listFiles, _ := filepath.Glob("/etc/apt/sources.list.d/*")
	paths = append(paths, listFiles...)
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "download.docker.com/linux/ubuntu") {
			continue
		}
		repo = true
		for _, line := range strings.Split(string(b), "\n") {
			lower := strings.ToLower(strings.TrimSpace(line))
			var keyPath string
			switch {
			case strings.HasPrefix(lower, "signed-by:"):
				keyPath = strings.TrimSpace(line[strings.Index(line, ":")+1:])
			case strings.Contains(lower, "signed-by="):
				i := strings.Index(lower, "signed-by=") + len("signed-by=")
				if fields := strings.Fields(line[i:]); len(fields) > 0 {
					keyPath = strings.Trim(fields[0], "[]\"'")
				}
			}
			if keyPath != "" && fileExists(keyPath) {
				key = true
			}
		}
	}
	return repo, key
}

func checkDocker(e *Env, rp *report) {
	have := dpkgInstalled(dockerPackages)
	var missing []string
	for _, p := range dockerPackages {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	repo, key := dockerRepoState()
	switch {
	case len(missing) > 0:
		rp.add("docker:packages", StatusWarn, "missing: %s", strings.Join(missing, " "))
	case !repo:
		rp.add("docker:packages", StatusWarn, "packages installed but Docker apt repository is missing")
	case !key:
		rp.add("docker:packages", StatusWarn, "Docker apt source has no readable Signed-By key")
	case exec.Command("docker", "buildx", "version").Run() != nil:
		rp.add("docker:packages", StatusWarn, "Buildx plugin does not run")
	case exec.Command("docker", "compose", "version").Run() != nil:
		rp.add("docker:packages", StatusWarn, "Compose plugin does not run")
	default:
		rp.add("docker:packages", StatusOK, "Engine, CLI, containerd, Buildx and Compose installed")
	}

	if os.Getenv("BOOTSTRAP_TEST_NO_SYSTEMD") == "1" {
		rp.add("docker:service", StatusInfo, "container test: systemd and daemon check skipped")
	} else {
		dockerEnabled := output("systemctl", "is-enabled", "docker.service") == "enabled"
		dockerActive := output("systemctl", "is-active", "docker.service") == "active"
		containerdEnabled := output("systemctl", "is-enabled", "containerd.service") == "enabled"
		containerdActive := output("systemctl", "is-active", "containerd.service") == "active"
		if dockerEnabled && dockerActive && containerdEnabled && containerdActive {
			rp.add("docker:service", StatusOK, "docker and containerd enabled and active")
		} else {
			var bad []string
			if !dockerEnabled || !dockerActive {
				bad = append(bad, "docker")
			}
			if !containerdEnabled || !containerdActive {
				bad = append(bad, "containerd")
			}
			rp.add("docker:service", StatusWarn, "%s not both enabled and active", strings.Join(bad, " and "))
		}
	}

	user := output("id", "-un")
	accountHasGroup := wordPresent(output("id", "-nG", user), "docker")
	currentHasGroup := wordPresent(output("id", "-nG"), "docker")
	if os.Getenv("BOOTSTRAP_TEST_NO_SYSTEMD") == "1" {
		if accountHasGroup {
			rp.add("docker:access", StatusInfo, "%s registered in docker group; daemon check skipped", user)
		} else {
			rp.add("docker:access", StatusWarn, "%s is not registered in the docker group", user)
		}
		return
	}
	if Which("docker") == "" {
		rp.add("docker:access", StatusWarn, "docker CLI is not installed")
		return
	}
	server := output("docker", "info", "--format", "{{.ServerVersion}}")
	status, detail := dockerAccessResult(user, accountHasGroup, currentHasGroup, server)
	rp.add("docker:access", status, "%s", detail)
}

func wordPresent(words, want string) bool {
	for _, word := range strings.Fields(words) {
		if word == want {
			return true
		}
	}
	return false
}

func dockerAccessResult(user string, accountHasGroup, currentHasGroup bool, server string) (Status, string) {
	switch {
	case !accountHasGroup:
		return StatusWarn, fmt.Sprintf("%s is not in the docker group", user)
	case !currentHasGroup:
		return StatusWarn, "docker group registered but not active; run newgrp docker or log in again"
	case server == "":
		return StatusWarn, "docker daemon API is not reachable without sudo"
	default:
		return StatusOK, fmt.Sprintf("daemon %s reachable without sudo", server)
	}
}

// ----------------------------------------------------------------------------
// locale
// ----------------------------------------------------------------------------

// zsh/.zshenv exports LANG=ja_JP.UTF-8, so a missing locale makes every
// subprocess warn. The subtlety worth keeping: LOCPATH makes glibc search that
// directory *exclusively* and stop consulting /usr/lib/locale, so a leftover
// per-user copy alongside a system locale masks it rather than supplementing it.
func checkLocale(e *Env, rp *report) {
	want := []string{"ja_JP.UTF-8", "en_US.UTF-8"}
	available := strings.ToLower(output("locale", "-a"))
	var missing []string
	for _, l := range want {
		base := strings.ToLower(strings.TrimSuffix(l, ".UTF-8"))
		if !strings.Contains(available, base+".utf8") && !strings.Contains(available, strings.ToLower(l)) {
			missing = append(missing, l)
		}
	}
	userDir := filepath.Join(e.XDGData, "locale")
	hasUserDir := fileExists(userDir)

	switch {
	case len(missing) > 0 && !hasUserDir:
		rp.add("locale", StatusFail, "missing: %s -- every subprocess will warn about LC_CTYPE",
			strings.Join(missing, " "))
	case len(missing) > 0:
		rp.add("locale", StatusOK, "via LOCPATH=%s", e.Tilde(userDir))
	case hasUserDir:
		rp.add("locale", StatusWarn,
			"system locales exist but %s does too; LOCPATH will mask them", e.Tilde(userDir))
	default:
		rp.add("locale", StatusOK, "%s system-wide", strings.Join(want, " "))
	}
}

// /etc/zsh/zshenv is the only file zsh reads before ZDOTDIR takes effect, so this
// is what makes the repo the shell's config at all.
func checkZdotdir(e *Env, rp *report) {
	const p = "/etc/zsh/zshenv"
	b, err := os.ReadFile(p)
	if err != nil {
		rp.add("zdotdir", StatusFail, "cannot read %s: %v", p, err)
		return
	}
	if strings.Contains(string(b), "ZDOTDIR") {
		rp.add("zdotdir", StatusOK, "set in %s", p)
		return
	}
	rp.add("zdotdir", StatusFail, "not set -- zsh will read ~/.zshrc instead of zsh/.zshrc")
}

// ----------------------------------------------------------------------------
// tools.tsv
// ----------------------------------------------------------------------------

func checkTools(e *Env, rp *report) {
	for _, t := range e.Tools {
		id := "tool:" + t.Name
		if t.IsFont {
			pattern := filepath.Join(e.FontDir, strings.Title(t.Name)+"*.ttf") //nolint:staticcheck // ASCII only
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				rp.add(id, StatusOK, "%d faces installed (%s pinned)", len(matches), t.Ref)
			} else {
				rp.add(id, StatusWarn, "font missing from %s", e.Tilde(e.FontDir))
			}
			continue
		}

		probe := t.Probe()
		where := Which(probe)
		if where == "" {
			rp.add(id, StatusWarn, "not installed (pinned %s)", t.Ref)
			continue
		}
		cur := InstalledVersion(probe)

		// Naming the directory matters: a copy outside ~/.local/bin means
		// something else -- usually an apt package -- is standing in for the
		// pinned one. On this machine eza, fzf, yazi and zoxide all came from a
		// third-party apt repo a fresh install knows nothing about.
		if !strings.HasPrefix(where, e.LocalBin+"/") {
			rp.add(id, StatusWarn, "%s from %s, not the pinned copy in %s",
				cur, where, e.Tilde(e.LocalBin))
			continue
		}

		switch {
		case t.Floating():
			if t.Min == "-" || AtLeast(cur, t.Min) {
				rp.add(id, StatusOK, "%s (floats; min %s)", cur, t.Min)
			} else {
				rp.add(id, StatusWarn, "%s is below the required %s", cur, t.Min)
			}
		case cur == t.Version():
			rp.add(id, StatusOK, "%s", cur)
		default:
			rp.add(id, StatusWarn, "pinned %s, installed %s", t.Version(), cur)
		}
	}
}

// ----------------------------------------------------------------------------
// runtimes
// ----------------------------------------------------------------------------

func checkRust(e *Env, rp *report) {
	r := e.Runtimes["rust"]
	if Which("cargo") == "" {
		rp.add("rust", StatusWarn, "not installed")
		return
	}
	cur := InstalledVersion("cargo")
	if AtLeast(cur, r.Min) {
		rp.add("rust", StatusOK, "%s (tracks stable, min %s)", cur, r.Min)
	} else {
		rp.add("rust", StatusWarn, "%s is below the required %s", cur, r.Min)
	}
}

func checkGo(e *Env, rp *report) {
	r := e.Runtimes["go"]
	where := Which("go")
	if where == "" {
		rp.add("go", StatusWarn, "not installed (pinned %s)", r.Ref)
		return
	}
	cur := InstalledVersion("go")
	// A go outside our own managed locations (the pinned tarball's GoRoot, or the
	// LocalBin symlink -- which may itself point at an apt copy that exactly
	// matched the pin, see step_go) is not something bs.sh controls, so its
	// version can drift out from under the pin on any apt upgrade.
	if !strings.HasPrefix(where, e.LocalBin+"/") && !strings.HasPrefix(where, e.GoRoot+"/") {
		rp.add("go", StatusWarn, "%s from %s (unmanaged copy); pinned %s", cur, where, r.Ref)
		return
	}
	if cur == r.Ref {
		rp.add("go", StatusOK, "%s", cur)
	} else {
		rp.add("go", StatusWarn, "pinned %s, installed %s", r.Ref, cur)
	}
}

// zsh/.zshenv finds node by globbing $NVM_DIR/versions/node/v<alias>*/bin rather
// than sourcing nvm.sh, so the default alias has to agree with the pin. When it
// does not, new shells quietly get a different node than the one running here.
func checkNode(e *Env, rp *report) {
	r := e.Runtimes["node"]
	if Which("node") == "" {
		rp.add("node", StatusWarn, "not installed (pinned %s)", r.Ref)
		return
	}
	cur := InstalledVersion("node")
	alias := ""
	if b, err := os.ReadFile(filepath.Join(e.NvmDir, "alias", "default")); err == nil {
		alias = strings.TrimSpace(string(b))
	}
	switch {
	case cur != r.Ref:
		rp.add("node", StatusWarn, "pinned %s, active %s", r.Ref, cur)
	case alias != r.Ref:
		rp.add("node", StatusWarn,
			"%s active, but nvm default alias is %q -- new shells may differ", cur, alias)
	default:
		rp.add("node", StatusOK, "%s (nvm default)", cur)
	}
}

// nodeGlobals must match NODE_GLOBALS in bootstrap/steps/50-runtimes.sh. The
// package name and the binary differ for tree-sitter-cli.
var nodeGlobals = map[string]string{
	"prettier":        "prettier",
	"eslint_d":        "eslint_d",
	"tree-sitter-cli": "tree-sitter",
}

func checkPnpm(e *Env, rp *report) {
	r := e.Runtimes["pnpm"]
	if Which("pnpm") == "" {
		rp.add("pnpm", StatusWarn, "not installed")
	} else if cur := InstalledVersion("pnpm"); cur == r.Ref {
		rp.add("pnpm", StatusOK, "%s", cur)
	} else {
		rp.add("pnpm", StatusWarn, "pinned %s, installed %s", r.Ref, cur)
	}

	var missing []string
	bins := make([]string, 0, len(nodeGlobals))
	for _, b := range nodeGlobals {
		bins = append(bins, b)
	}
	sort.Strings(bins)
	for _, bin := range bins {
		where := Which(bin)
		if where == "" {
			missing = append(missing, bin)
			continue
		}
		switch {
		// On PATH and broken. tree-sitter-cli's npm package is a JS wrapper around
		// a native binary its postinstall downloads, and pnpm 10 does not run build
		// scripts for unapproved packages -- so the wrapper installs, every call
		// dies with ENOENT, and checking the path alone passed an editor that could
		// not build a single parser.
		case !Runs(bin):
			rp.add("node:"+bin, StatusWarn,
				"%s exists but does not run (fix: bs.sh --only pnpm)", e.Tilde(where))
		// Inside a single node version's directory it works today and vanishes
		// on the next node bump, taking conform.nvim's formatter with it.
		case strings.HasPrefix(where, e.NvmDir+"/"):
			rp.add("node:"+bin, StatusWarn,
				"installed under a single node version (%s); bumping node loses it", where)
		default:
			rp.add("node:"+bin, StatusOK, "%s", e.Tilde(where))
		}
	}
	if len(missing) > 0 {
		rp.add("node:tools", StatusWarn,
			"missing: %s (conform.nvim / nvim-lint / nvim-treesitter need these)",
			strings.Join(missing, " "))
	}
}

// npm-globals.tsv rows are protected against a node bump the same way
// nodeGlobals is above (installed into $PNPM_HOME, not $NVM_DIR), but each row
// also carries its own version pin, so this additionally compares against Ref
// the way checkTools does for tools.tsv.
func checkNpmGlobals(e *Env, rp *report) {
	for _, g := range e.NpmGlobals {
		id := "npm:" + g.Name
		bin := g.Probe()
		where := Which(bin)
		switch {
		case where == "":
			rp.add(id, StatusWarn, "not installed (pinned %s)", g.Ref)
		case !Runs(bin):
			rp.add(id, StatusWarn, "%s exists but does not run (fix: bs.sh --only %s)", e.Tilde(where), id)
		case strings.HasPrefix(where, e.NvmDir+"/"):
			rp.add(id, StatusWarn,
				"installed under a single node version (%s); bumping node loses it", where)
		case g.Floating():
			rp.add(id, StatusOK, "%s (floats)", InstalledVersion(bin))
		default:
			if cur := InstalledVersion(bin); cur == strings.TrimPrefix(g.Ref, "v") {
				rp.add(id, StatusOK, "%s", cur)
			} else {
				rp.add(id, StatusWarn, "pinned %s, installed %s", g.Ref, cur)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// neovim
// ----------------------------------------------------------------------------

func checkNvim(e *Env, rp *report) {
	r := e.Runtimes["nvim"]
	if Which("nvim") == "" {
		rp.add("nvim", StatusWarn, "not installed (pinned %s)", r.Ref)
	} else if cur := InstalledVersion("nvim"); cur == strings.TrimPrefix(r.Ref, "v") {
		rp.add("nvim", StatusOK, "%s (%s)", cur, e.Tilde(Which("nvim")))
	} else {
		rp.add("nvim", StatusWarn, "pinned %s, installed %s", strings.TrimPrefix(r.Ref, "v"), cur)
	}

	// Plugins: the lockfile is the desired state.
	lock := filepath.Join(e.ConfigDir, "nvim", "lazy-lock.json")
	var locked map[string]any
	if b, err := os.ReadFile(lock); err == nil {
		_ = json.Unmarshal(b, &locked)
	}
	installed, _ := filepath.Glob(filepath.Join(e.XDGData, "nvim", "lazy", "*"))
	switch {
	case len(locked) == 0:
		rp.add("nvim:plugins", StatusWarn, "lazy-lock.json missing or unreadable")
	case len(installed) == 0:
		rp.add("nvim:plugins", StatusWarn, "no plugins installed (%d in the lockfile)", len(locked))
	case len(installed) < len(locked):
		rp.add("nvim:plugins", StatusWarn, "%d of %d plugins installed", len(installed), len(locked))
	default:
		rp.add("nvim:plugins", StatusOK, "%d plugins", len(installed))
	}

	// Parsers are a separate row on purpose. nvim-treesitter's rewritten branch
	// shells out to `tree-sitter build`; when that binary is absent every build
	// fails while the plugin tree still looks complete, so counting plugins alone
	// reported a healthy editor with no syntax highlighting at all.
	parsers, _ := filepath.Glob(filepath.Join(e.XDGData, "nvim", "site", "parser", "*.so"))
	wantParsers := countParserList(filepath.Join(e.ConfigDir, "nvim", "lua", "config", "treesitter-parsers.lua"))
	switch {
	case len(parsers) == 0:
		rp.add("nvim:parsers", StatusWarn,
			"none built (check the node:tree-sitter row, then bs.sh --only nvim:plugins)")
	case wantParsers > 0 && len(parsers) < wantParsers:
		rp.add("nvim:parsers", StatusWarn, "%d built, %d requested", len(parsers), wantParsers)
	default:
		rp.add("nvim:parsers", StatusOK, "%d parsers", len(parsers))
	}

	// The binaries conform.nvim and nvim-lint invoke by name. Which package
	// manager provided them does not matter; being on PATH does.
	var missing []string
	for _, bin := range []string{"clangd", "clang-format", "stylua", "rustfmt"} {
		if Which(bin) == "" {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		rp.add("nvim:tools", StatusWarn, "not on PATH: %s", strings.Join(missing, " "))
	} else {
		rp.add("nvim:tools", StatusOK, "clangd, clang-format, stylua, rustfmt")
	}

	// A lockfile with uncommitted changes means this machine has drifted ahead of
	// what a fresh install would reproduce.
	if fileExists(lock) {
		cmd := exec.Command("git", "-C", e.ConfigDir, "diff", "--quiet", "--", "nvim/lazy-lock.json")
		if err := cmd.Run(); err != nil {
			rp.add("nvim:lock", StatusWarn, "uncommitted plugin updates -- commit nvim/lazy-lock.json")
		} else {
			rp.add("nvim:lock", StatusOK, "committed")
		}
	}
}

func countParserList(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), `"`) {
			n++
		}
	}
	return n
}

// ----------------------------------------------------------------------------
// git
// ----------------------------------------------------------------------------

func checkGit(e *Env, rp *report) {
	local := filepath.Join(e.ConfigDir, "git", "config.local")
	if !fileExists(local) {
		rp.add("git:local", StatusWarn, "git/config.local missing, but git/config includes it")
	} else if email := output("git", "-C", e.ConfigDir, "config", "--get", "user.email"); email != "" {
		rp.add("git:local", StatusOK, "identity resolves to %s", email)
	} else {
		// git/config sets user.useConfigOnly, so an unset identity is not a
		// warning at commit time -- it is a refusal.
		rp.add("git:local", StatusFail, "no user.email resolves -- commits will refuse to run")
	}

	// ~/.gitconfig is read after the XDG file and wins, so anything in it is
	// invisible to this repo and absent on a fresh machine.
	home := filepath.Join(e.Home, ".gitconfig")
	if fileExists(home) {
		// Name the settings that are only there, not every key -- those are the
		// ones a fresh machine silently would not get.
		var only []string
		for _, key := range []string{"user.signingkey", "commit.gpgsign", "user.email"} {
			if output("git", "config", "--file", home, "--get", key) != "" {
				only = append(only, key)
			}
		}
		rp.add("git:home", StatusWarn,
			"~/.gitconfig overrides git/config; it sets %s -- fold them into git/config.local",
			strings.Join(only, ", "))
	} else {
		rp.add("git:home", StatusOK, "no ~/.gitconfig shadowing the XDG config")
	}
}

// sheldon's own lockfile lives outside the repo and records no revisions, so the
// rev pins in plugins.toml are the only thing that makes zsh plugins reproducible.
func checkZshPlugins(e *Env, rp *report) {
	if len(e.Plugins) == 0 {
		rp.add("zsh:plugins", StatusWarn, "no pinned plugins found in sheldon/plugins.toml")
		return
	}
	var unpinned, missing, mismatched []string
	for _, p := range e.Plugins {
		if p.Rev == "" {
			unpinned = append(unpinned, p.Short())
			continue
		}
		dir := filepath.Join(e.XDGData, "sheldon", "repos", "github.com", p.Repo)
		if !fileExists(dir) {
			missing = append(missing, p.Short())
			continue
		}
		if output("git", "-C", dir, "rev-parse", "HEAD") != p.Rev {
			mismatched = append(mismatched, p.Short())
		}
	}
	switch {
	case len(unpinned) > 0:
		rp.add("zsh:plugins", StatusWarn, "unpinned: %s", strings.Join(unpinned, " "))
	case len(missing) > 0:
		rp.add("zsh:plugins", StatusWarn, "not cloned: %s (run: sheldon lock)", strings.Join(missing, " "))
	case len(mismatched) > 0:
		rp.add("zsh:plugins", StatusWarn,
			"at a different revision: %s (run: sheldon lock)", strings.Join(mismatched, " "))
	default:
		rp.add("zsh:plugins", StatusOK, "all %d plugins at their pinned revisions", len(e.Plugins))
	}
}

func checkShell(e *Env, rp *report) {
	user := output("id", "-un")
	line := output("getent", "passwd", user)
	fields := strings.Split(line, ":")
	if len(fields) < 7 {
		rp.add("shell", StatusWarn, "cannot determine the login shell for %s", user)
		return
	}
	current := fields[6]
	zsh := Which("zsh")
	if zsh == "" {
		rp.add("shell", StatusWarn, "zsh is not installed")
		return
	}
	// /bin is a symlink to /usr/bin on Ubuntu, so compare resolved paths --
	// /bin/zsh and /usr/bin/zsh are the same shell.
	if realpath(current) == realpath(zsh) {
		rp.add("shell", StatusOK, "%s", current)
	} else {
		rp.add("shell", StatusWarn, "login shell is %s, not %s", current, zsh)
	}
}

func realpath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// lib.sh's path_init() is a hand-kept copy of the path=(...) list in
// zsh/.zshenv. What breaks when they drift is bootstrap installing into a
// directory the shell never looks at, so assert exactly that: every directory
// bootstrap uses must be on zsh's $path. Not set equality -- zsh's $path
// legitimately has more, inherited from /etc/profile and friends.
//
// The list comes from `bs.sh --print-path-contract` rather than being duplicated
// here a third time.
func checkPathContract(e *Env, rp *report) {
	bs := filepath.Join(e.BootstrapDir, "bs.sh")
	want := strings.Fields(output(bs, "--print-path-contract"))
	if len(want) == 0 {
		rp.add("path", StatusWarn, "could not read the PATH contract from bs.sh")
		return
	}
	zpath := output("zsh", "-c", "print -rl -- $path")
	if zpath == "" {
		rp.add("path", StatusWarn, "could not read zsh $path")
		return
	}
	inZsh := map[string]bool{}
	for _, d := range strings.Split(zpath, "\n") {
		inZsh[strings.TrimSpace(d)] = true
	}
	var missing []string
	for _, d := range want {
		if !fileExists(d) {
			continue // absent directories cost nothing; .zshenv filters them too
		}
		if !inZsh[d] {
			missing = append(missing, e.Tilde(d))
		}
	}
	if len(missing) > 0 {
		rp.add("path", StatusWarn, "missing from zsh $path (add to zsh/.zshenv): %s",
			strings.Join(missing, " "))
	} else {
		rp.add("path", StatusOK, "all %d bootstrap dirs are on zsh $path", len(want))
	}
}

// WSL appends ~16 Windows directories to PATH and zsh/.zshenv prunes them, which
// is worth ~80ms per shell. Reported so a regression of that is visible; never a
// warning, since the number is a preference.
func checkWSL(e *Env, rp *report) {
	if !isWSL() {
		return
	}
	n := 0
	for _, d := range strings.Split(output("zsh", "-c", "print -rl -- $path"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(d), "/mnt/") {
			n++
		}
	}
	rp.add("wsl", StatusInfo, "%d Windows PATH entries kept (pruned in zsh/.zshenv)", n)
}
