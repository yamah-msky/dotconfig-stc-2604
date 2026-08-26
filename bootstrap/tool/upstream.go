package main

// Resolving the latest version of everything pinned. Only `update` does this --
// installing computes its URLs from the committed manifests and never calls an
// API, which is what keeps a bootstrap run deterministic and away from GitHub's
// 60-requests-per-hour anonymous limit.
//
// The five sources are queried concurrently: serially they took several seconds
// of pure waiting.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func httpGet(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// LatestReleaseTag returns the tag of a repository's latest release, verbatim --
// keeping the v prefix when upstream uses one, because the manifest stores tags
// as published.
//
// Prefers `gh api`: it is authenticated, so it gets 5000 requests/hour instead of
// 60 shared per IP, which matters when resolving a dozen repositories at once.
func LatestReleaseTag(repo string) (string, error) {
	if Which("gh") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		out, err := exec.CommandContext(ctx, "gh", "api", "repos/"+repo+"/releases/latest", "--jq", ".tag_name").Output()
		cancel()
		if err == nil {
			if tag := strings.TrimSpace(string(out)); tag != "" {
				return tag, nil
			}
		}
	}
	body, err := httpGet("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	var v struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.TagName == "" {
		return "", fmt.Errorf("%s: no tag_name in the latest release", repo)
	}
	return v.TagName, nil
}

// LatestNodeLTS returns the newest LTS release, without the v prefix, to match
// how runtimes.tsv stores it.
func LatestNodeLTS() (string, error) {
	body, err := httpGet("https://nodejs.org/dist/index.json")
	if err != nil {
		return "", err
	}
	var releases []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"` // false, or the codename as a string
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", err
	}
	// The index is newest first.
	for _, r := range releases {
		if string(r.LTS) != "false" {
			return strings.TrimPrefix(r.Version, "v"), nil
		}
	}
	return "", fmt.Errorf("no LTS release found in the node index")
}

func LatestPnpm() (string, error) {
	body, err := httpGet("https://registry.npmjs.org/pnpm")
	if err != nil {
		return "", err
	}
	var v struct {
		DistTags struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.DistTags.Latest == "" {
		return "", fmt.Errorf("no dist-tags.latest for pnpm")
	}
	return v.DistTags.Latest, nil
}

// LatestNpmPackage returns the npm registry's "latest" dist-tag for any
// package, scoped or not -- the same endpoint and field LatestPnpm reads for
// pnpm itself, generalised for npm-globals.tsv. Scoped names need their slash
// percent-encoded (registry.npmjs.org/@scope%2Fname); PathEscape leaves the
// leading @ alone and encodes exactly that slash.
func LatestNpmPackage(pkg string) (string, error) {
	body, err := httpGet("https://registry.npmjs.org/" + url.PathEscape(pkg))
	if err != nil {
		return "", err
	}
	var v struct {
		DistTags struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.DistTags.Latest == "" {
		return "", fmt.Errorf("no dist-tags.latest for %s", pkg)
	}
	return v.DistTags.Latest, nil
}

// LatestGo returns e.g. "1.26.5"; the endpoint answers "go1.26.5".
func LatestGo() (string, error) {
	body, err := httpGet("https://go.dev/VERSION?m=text")
	if err != nil {
		return "", err
	}
	first := strings.TrimSpace(strings.SplitN(string(body), "\n", 2)[0])
	if !strings.HasPrefix(first, "go") {
		return "", fmt.Errorf("unexpected reply from go.dev: %q", first)
	}
	return strings.TrimPrefix(first, "go"), nil
}

// RemoteHead is the commit a branchless `git ls-remote HEAD` reports, used for
// the sheldon plugin pins.
func RemoteHead(repo string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "https://github.com/"+repo, "HEAD").Output()
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return "", fmt.Errorf("%s: empty ls-remote reply", repo)
	}
	return f[0], nil
}

// resolution is one answer from upstream, successful or not.
type resolution struct {
	key    string // manifest row name, or plugin repo
	latest string
	err    error
}

// resolveAll runs the given lookups concurrently and returns them keyed by name.
// Failures are carried rather than aborting: one unreachable upstream should not
// stop the other eleven from being reported.
func resolveAll(jobs map[string]func() (string, error)) map[string]resolution {
	var wg sync.WaitGroup
	ch := make(chan resolution, len(jobs))
	// A small ceiling: a dozen simultaneous requests to the same host is impolite
	// and gains nothing over four.
	sem := make(chan struct{}, 4)

	for key, fn := range jobs {
		wg.Add(1)
		go func(key string, fn func() (string, error)) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			v, err := fn()
			ch <- resolution{key: key, latest: v, err: err}
		}(key, fn)
	}
	wg.Wait()
	close(ch)

	out := make(map[string]resolution, len(jobs))
	for r := range ch {
		out[r.key] = r
	}
	return out
}
