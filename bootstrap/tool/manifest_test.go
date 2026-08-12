package main

import (
	"os"
	"path/filepath"
	"testing"
)

// repoDir is the real bootstrap/ directory. Testing against the actual manifests
// rather than fixtures means a malformed row fails `go test`, not a bootstrap run
// on a fresh machine.
func repoDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tools.tsv")); err != nil {
		t.Skipf("not running inside the repository: %v", err)
	}
	return dir
}

func TestLoadRealManifests(t *testing.T) {
	dir := repoDir(t)

	tools, err := LoadTools(dir)
	if err != nil {
		t.Fatalf("tools.tsv: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("tools.tsv parsed to zero rows")
	}
	seen := map[string]bool{}
	for _, tl := range tools {
		if seen[tl.Name] {
			t.Errorf("tools.tsv has two rows named %q", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Repo == "" || tl.Asset == "" {
			t.Errorf("%s: empty repo or asset", tl.Name)
		}
		if !tl.IsFont && len(tl.Bins) == 0 {
			t.Errorf("%s: no binaries and not a font", tl.Name)
		}
	}

	runtimes, err := LoadRuntimes(dir)
	if err != nil {
		t.Fatalf("runtimes.tsv: %v", err)
	}
	for _, r := range runtimes {
		if r.Ref == "" {
			t.Errorf("runtimes.tsv %s: empty ref", r.Name)
		}
	}

	groups, err := LoadAptGroups(dir)
	if err != nil {
		t.Fatalf("apt.tsv: %v", err)
	}
	for _, g := range groups {
		if len(g.Packages) == 0 {
			t.Errorf("apt.tsv group %q has no packages", g.Name)
		}
	}
	// A group spanning several lines must merge, not overwrite.
	for _, g := range groups {
		if g.Name == "base" && len(g.Packages) < 10 {
			t.Errorf("apt.tsv base has only %d packages; multi-line groups may not be merging",
				len(g.Packages))
		}
	}

	npmGlobals, err := LoadNpmGlobals(dir)
	if err != nil {
		t.Fatalf("npm-globals.tsv: %v", err)
	}
	if len(npmGlobals) == 0 {
		t.Fatal("npm-globals.tsv parsed to zero rows")
	}
	seenNpm := map[string]bool{}
	for _, g := range npmGlobals {
		if seenNpm[g.Name] {
			t.Errorf("npm-globals.tsv has two rows named %q", g.Name)
		}
		seenNpm[g.Name] = true
		if g.Package == "" || g.Ref == "" {
			t.Errorf("%s: empty package or ref", g.Name)
		}
		if g.Probe() == "" {
			t.Errorf("%s: Probe() returned empty", g.Name)
		}
	}
}

// Every placeholder must resolve on a supported arch. An unexpanded {BRACE} in a
// URL is a 404 halfway through a download, so catch it here instead.
func TestAssetNamesFullyExpand(t *testing.T) {
	dir := repoDir(t)
	tools, err := LoadTools(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, uname := range []string{"x86_64", "aarch64"} {
		a := ArchFor(uname)
		for _, tl := range tools {
			name := tl.AssetName(a)
			for _, ch := range []string{"{", "}"} {
				if contains(name, ch) {
					t.Errorf("%s on %s: unexpanded placeholder in %q", tl.Name, uname, name)
				}
			}
		}
	}
}

// Asset names verified against the live releases, so a template edit that would
// produce a 404 fails the test suite.
func TestKnownAssetNames(t *testing.T) {
	dir := repoDir(t)
	tools, err := LoadTools(dir)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Tool{}
	for _, tl := range tools {
		byName[tl.Name] = tl
	}
	a := ArchFor("x86_64")

	cases := map[string]string{
		"eza":    "eza_x86_64-unknown-linux-gnu.tar.gz",
		"fzf":    "fzf-0.74.2-linux_amd64.tar.gz",
		"zoxide": "zoxide-0.10.0-x86_64-unknown-linux-musl.tar.gz", // musl only; no gnu asset exists
		"yazi":   "yazi-x86_64-unknown-linux-gnu.zip",
		"bob":    "bob-linux-x86_64.zip",
		"cica":   "Cica_v5.0.3_without_emoji.zip", // {TAG} keeps the v
		"stylua": "stylua-linux-x86_64-musl.zip",
	}
	for name, want := range cases {
		tl, ok := byName[name]
		if !ok {
			t.Errorf("tools.tsv no longer has a row for %q", name)
			continue
		}
		if got := tl.AssetName(a); got != want {
			t.Errorf("%s asset = %q, want %q", name, got, want)
		}
	}
}

// Tags with and without a v prefix must both produce a valid download URL.
func TestDownloadURL(t *testing.T) {
	a := ArchFor("x86_64")
	cases := []struct {
		tool Tool
		want string
	}{
		{
			Tool{Name: "eza", Ref: "v0.23.5", Repo: "eza-community/eza",
				Asset: "eza_{RUST_GNU}.tar.gz", Bins: []string{"eza"}},
			"https://github.com/eza-community/eza/releases/download/v0.23.5/eza_x86_64-unknown-linux-gnu.tar.gz",
		},
		{
			// sheldon publishes "0.8.5", with no v.
			Tool{Name: "sheldon", Ref: "0.8.5", Repo: "rossmacarthur/sheldon",
				Asset: "sheldon-{VERSION}-{RUST_MUSL}.tar.gz", Bins: []string{"sheldon"}},
			"https://github.com/rossmacarthur/sheldon/releases/download/0.8.5/sheldon-0.8.5-x86_64-unknown-linux-musl.tar.gz",
		},
		{
			// A floating row uses the /latest/download/ form, which needs no API call.
			Tool{Name: "yt-dlp", Ref: "latest", Repo: "yt-dlp/yt-dlp",
				Asset: "yt-dlp_linux", Bins: []string{"yt-dlp"}},
			"https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux",
		},
	}
	for _, c := range cases {
		if got := c.tool.DownloadURL(a); got != c.want {
			t.Errorf("%s URL:\n got %q\nwant %q", c.tool.Name, got, c.want)
		}
	}
}

func TestArchDegradesInsteadOfGuessing(t *testing.T) {
	if ArchFor("riscv64").Supported() {
		t.Error("riscv64 reported as supported; it has no prebuilt assets")
	}
	if !ArchFor("x86_64").Supported() {
		t.Error("x86_64 reported as unsupported")
	}
	// bob names its ARM asset "arm", not "aarch64".
	if got := ArchFor("aarch64").BobArch; got != "arm" {
		t.Errorf("aarch64 BobArch = %q, want %q", got, "arm")
	}
}

// The commented-out example in plugins.toml must not be read as a plugin, and
// every real plugin must carry a rev.
func TestLoadPlugins(t *testing.T) {
	dir := filepath.Dir(repoDir(t))
	plugins, err := LoadPlugins(dir)
	if err != nil {
		t.Fatalf("plugins.toml: %v", err)
	}
	if len(plugins) == 0 {
		t.Fatal("no plugins parsed")
	}
	for _, p := range plugins {
		if p.Repo == "chriskempson/base16-shell" {
			t.Error("picked up the commented-out example from plugins.toml")
		}
		if p.Rev == "" {
			t.Errorf("plugin %s has no rev pin", p.Repo)
		}
		if len(p.Rev) != 40 {
			t.Errorf("plugin %s rev %q is not a full commit sha", p.Repo, p.Rev)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
