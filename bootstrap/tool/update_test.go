package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTSVChangesAppliesSeveralRowsOnce(t *testing.T) {
	in := []byte("# header\na\t1\told\nb\t2\told\n")
	changes := []manifestChange{
		{key: "a", column: 2, to: "10"},
		{key: "b", column: 3, to: "new"},
	}
	got, err := renderTSVChanges("test.tsv", in, changes)
	if err != nil {
		t.Fatal(err)
	}
	want := "# header\na\t10\told\nb\t2\tnew\n"
	if got != want {
		t.Fatalf("renderTSVChanges() = %q, want %q", got, want)
	}
}

func TestRenderTSVChangesRejectsMissingRow(t *testing.T) {
	_, err := renderTSVChanges("test.tsv", []byte("a\t1\n"),
		[]manifestChange{{key: "missing", column: 2, to: "2"}})
	if err == nil || !strings.Contains(err.Error(), "no row named") {
		t.Fatalf("error = %v, want missing-row error", err)
	}
}

func TestRenderPluginRevsIsAllOrNothing(t *testing.T) {
	in := []byte("[plugins.a]\ngithub = \"owner/a\"\nrev = \"old-a\"\n")
	if _, err := renderPluginRevs(in, map[string]string{"owner/missing": "new"}); err == nil {
		t.Fatal("missing plugin revision unexpectedly succeeded")
	}
	got, err := renderPluginRevs(in, map[string]string{"owner/a": "new-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `rev = "new-a"`) {
		t.Fatalf("rendered plugin file did not contain new revision: %s", got)
	}
}

func TestApplyManifestChangesValidatesBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	bootstrap := filepath.Join(dir, "bootstrap")
	if err := os.MkdirAll(bootstrap, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bootstrap, "tools.tsv")
	original := "a\t1\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Env{BootstrapDir: bootstrap, ConfigDir: dir}
	err := applyManifestChanges(e, []manifestChange{
		{file: "tools.tsv", key: "a", column: 2, to: "2"},
		{file: "tools.tsv", key: "missing", column: 2, to: "3"},
	}, nil)
	if err == nil {
		t.Fatal("invalid change unexpectedly succeeded")
	}
	b, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(b) != original {
		t.Fatalf("file changed after validation failure: %q", b)
	}
}

func TestUpdateResolutionFailureWritesNothing(t *testing.T) {
	dir := t.TempDir()
	bootstrap := filepath.Join(dir, "bootstrap")
	if err := os.MkdirAll(bootstrap, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bootstrap, "tools.tsv")
	original := "tool\tv1.0.0\t-\towner/repo\ttool.tar.gz\tbin:tool\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	old := resolveLatestRelease
	resolveLatestRelease = func(string) (string, error) { return "", os.ErrDeadlineExceeded }
	t.Cleanup(func() { resolveLatestRelease = old })

	e := &Env{
		BootstrapDir: bootstrap,
		ConfigDir:    dir,
		Tools: []Tool{{Name: "tool", Ref: "v1.0.0", Repo: "owner/repo",
			Asset: "tool.tar.gz", Bins: []string{"tool"}}},
		Runtimes: map[string]Runtime{},
	}
	if rc := Update(e, []string{"tool"}, false); rc != 1 {
		t.Fatalf("Update() exit = %d, want 1", rc)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("manifest changed after resolution failure: %q", b)
	}
}
