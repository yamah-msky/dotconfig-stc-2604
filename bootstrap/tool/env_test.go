package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The distinction Which cannot make: a wrapper script whose native binary was
// never fetched is on PATH, is executable, and fails on every call. That is what
// `pnpm add -g tree-sitter-cli` produced with build scripts blocked, and reading
// only the path reported it as a working tool.
func TestRuns(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("goodtool", "#!/bin/sh\necho 1.2.3\n")
	write("brokentool", "#!/bin/sh\necho 'Error: spawn ... ENOENT' >&2\nexit 1\n")

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cases := []struct {
		bin  string
		want bool
	}{
		{"goodtool", true},
		{"brokentool", false}, // on PATH, executable, does not work
		{"nosuchtool_xyz", false},
	}
	for _, c := range cases {
		if got := Runs(c.bin); got != c.want {
			t.Errorf("Runs(%q) = %v, want %v", c.bin, got, c.want)
		}
	}
	if Which("brokentool") == "" {
		t.Error("Which(brokentool) = \"\"; the fixture is meant to be found on PATH")
	}
}

func TestCommandTimeout(t *testing.T) {
	old := localCommandTimeout
	localCommandTimeout = 25 * time.Millisecond
	t.Cleanup(func() { localCommandTimeout = old })

	start := time.Now()
	if commandRuns("sh", "-c", "sleep 2") {
		t.Fatal("timed-out command unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}
