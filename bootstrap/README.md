# bootstrap

**詳細マニュアルは [../manual/index.html](../manual/index.html)、端末用の早見表は
[MANUAL.md](MANUAL.md) にあります。** このファイルは設計の記録で、
なぜこうなっているかを説明するもの。

Reproduce this `~/.config` on an Ubuntu machine, and keep its pinned versions
current. Two commands do almost everything:

```sh
~/.config/bootstrap/bs.sh          # set up, or repair, the machine
~/.config/bootstrap/bs.sh update   # bump every pin, then review the git diff
```

Both are idempotent and safe to re-run.

## Commands

| | |
| --- | --- |
| `bs.sh` | run every step in order (this is `bs.sh install`) |
| `bs.sh update` | resolve the latest version of everything pinned and rewrite the manifests |
| `bs.sh doctor` | report where this machine differs from the manifests; exits 1 if it does |
| `bs.sh list` | list the steps, and which need root |

Options: `--only ID,…` and `--skip ID,…` (prefix match, so `--only tool:` picks
every tool), `--dry-run`, `--no-sudo`, `--arch ARCH`, and `--check` for `update`.

```sh
bs.sh --only tool:              # reinstall the pinned binaries
bs.sh --only tool:fzf           # just one
bs.sh --skip nvim:plugins       # skip the slow one
bs.sh update --check            # is anything behind upstream?
bs.sh update starship           # bump one thing
```

## Updating versions

```sh
bs.sh update       # rewrites the manifests
git diff           # see exactly what moved
git commit -am 'Bump pins'
bs.sh              # install it
```

Nothing is bumped behind your back: `bs.sh` only ever installs what the
committed manifests say. `update` is the one command that changes them, and it
leaves the result as a reviewable diff.

## Layout

```
bs.sh             the driver: argument parsing, step dispatch, install
lib.sh            everything the steps share (logging, versions, downloads, PATH)
tools.tsv         prebuilt binaries from GitHub releases -> ~/.local/bin
apt.tsv           apt package set, by group
runtimes.tsv      nvim / node / pnpm / go / rust, each via its own manager
npm-globals.tsv   npm/pnpm global CLIs -> $PNPM_HOME, independent of nvm's node
steps/*.sh        one file per concern, run in filename order
tool/             doctor and update, in Go (see below)
cleanup.sh        retire files that are no longer read (opt-in, moves not deletes)
test/             lint.sh, and the whole thing in a clean ubuntu:26.04 container
```

### Why two languages

`install` is shell because it is almost entirely process orchestration — there
are over a hundred places this shells out to apt, curl, tar, git or nvim, and in
Go each of those becomes `exec.Command` plus error handling. More importantly, a
bootstrap cannot require the toolchain it installs: this repository has no CI and
does not push, so a Go binary would have to be built on the fresh machine, by the
EOL Go 1.22 that `runtimes.tsv` exists to replace.

`doctor` and `update` are the opposite: HTTP, JSON, and version comparison. That
is where the shell version's real bugs lived — `0.9.3` comparing above `0.10.0`,
tags with and without a `v` prefix, asset names expanding wrong — so they are in
Go where `go test` can pin the behaviour down. Neither is needed on a fresh
machine (nothing is behind upstream on day one), so by the time you run them
`bs.sh` has installed Go.

`bs.sh doctor` and `bs.sh update` build `tool/` on first use and cache the binary
in `$XDG_CACHE_HOME/dotconfig/`, rebuilding when a `.go` file changes. The binary
is never committed: it would be per-architecture and unverifiable by reading. The
package has **no dependencies**, so the build needs no network and no `go.sum`.

The cost of this split is that a step's install lives in `steps/*.sh` while its
verification lives in `tool/doctor.go`. Manifest-driven rows are unaffected —
`tools.tsv` still generates both — but the eight bespoke steps now have two homes.

### Adding a tool

If it is a binary from a GitHub release, add one line to `tools.tsv`. Nothing
else — the step, the `--only` id, the doctor row and `update` support all follow
from the manifest.

```
name	ref	min	repo	asset	install
```

`ref` is the upstream release tag **verbatim**, because upstreams disagree about
the `v` prefix (sheldon and uv publish `0.8.5`, everyone else `v0.8.5`).
`{VERSION}` in `asset` is that tag with a leading `v` stripped; the other
placeholders are listed at the top of the file.

Anything more involved than "download and unpack" gets a step in `steps/`. A step
is a shell function plus a one-line `register` call; see `steps/30-zdotdir.sh`
for the smallest example.

If it is a CLI you install globally via npm/pnpm (a coding-agent tool, a
linter, anything not needed by this repo's own configs but that you use day to
day), add one line to `npm-globals.tsv` instead. `npm install -g` puts a
package inside `$NVM_DIR/versions/node/vX.Y.Z/lib/node_modules` -- one node
version's own directory -- and `zsh/.zshenv` only ever PATHs the version
`nvm alias default` currently points at (see `lib.sh`'s `path_init`), so the
package silently drops off PATH the next time `bs.sh --only node` bumps that
alias to the pin in `runtimes.tsv`. It looks deleted; it is actually just
stranded under the old version. `npm-globals.tsv` installs the same package via
`pnpm add -g` into `$PNPM_HOME` instead, which stays on PATH regardless of
which node is current -- the same fix `steps/50-runtimes.sh`'s `NODE_GLOBALS`
already applies to prettier and eslint_d, generalised to a manifest so adding
one is as cheap as adding a row to `tools.tsv`.

## Why these versions are pinned, and these are not

**Pinned** — the release binaries, nvim, node, pnpm, go, the zsh plugins
(`sheldon/plugins.toml`), the neovim plugin tree (`nvim/lazy-lock.json`), and
`npm-globals.tsv` rows (unless a row says `latest`, the same opt-out `tools.tsv`
gives yt-dlp).
These are what the configuration actually invokes; a surprise major bump changes
the prompt, the keybindings or the plugin loader. Pinning them is cheap because
`bs.sh update` bumps them all at once. `go` is the one exception with a shortcut:
`step_go` (`steps/50-runtimes.sh`) will link an apt-installed `/usr/lib/go-*/bin/go`
instead of downloading, but only when its version exactly matches the pin —
apt's patch release is not something this repo controls, so it is never trusted
to satisfy the pin approximately.

**Not pinned, deliberately:**

- **apt packages.** Ubuntu's archive removes superseded versions, so
  `ripgrep=14.1.1-1` starts failing the moment resolute-updates rotates the
  package. A pin here would reduce reproducibility. `apt.tsv` fixes the *set*.
- **yt-dlp.** An old yt-dlp fails against site changes. `latest` is the
  reproducible choice here, and it says so explicitly in the manifest.
- **rust.** Nothing here depends on a rustc version, and re-downloading a ~200MB
  toolchain per bump is not worth it. `min` still catches an ancient one.

**Known limit: Mason's LSP servers are not pinned.** `mason.nvim` has no
lockfile, so the servers `nvim/lua/config/plugins/lsp.lua` asks for install at
whatever version Mason's registry offers that day. Pinning them would mean
`ensure_installed = { "lua_ls@3.7.4" }` or adding a plugin like
`mason-lock.nvim`. Treesitter parsers *are* effectively pinned, because they
compile from the `nvim-treesitter` commit in `lazy-lock.json`.

## Root

Four steps need it: `apt:base`, `apt:cpp`, `apt:media`, `gh`, `locale`,
`zdotdir` and `shell`. They elevate individual commands; **do not run `bs.sh`
itself under sudo** — it would install into `/root/.local`, and it refuses.

`--no-sudo` skips them and everything else still runs, then prints the command to
finish the job later. The locale step also has a rootless path: it compiles into
`$XDG_DATA_HOME/locale`, which `zsh/.zshenv` picks up via `LOCPATH`.

## Testing

```sh
bootstrap/test/lint.sh             # bash -n, shellcheck, gofmt, go vet, go test
bs.sh --dry-run                    # prints every mutation, performs none
bs.sh --dry-run --arch riscv64     # unsupported arch must skip, not fail
bootstrap/test/container.sh        # the real one; --fast skips the nvim plugins
```

`lint.sh` falls back to shellcheck's container image when shellcheck is not
installed yet, which is exactly when you most want to run it. It also checks the
Go tool still builds with `/usr/lib/go-1.26/bin/go` — Ubuntu 26.04's Go — since
that is what a fresh machine has.

**Presence on PATH is not health.** `has` (shell) and `Which` (Go) answer "is
there a file", which an npm wrapper whose native binary was never fetched
satisfies perfectly while failing on every call — that is how a container run
produced an editor with 34 plugins and zero treesitter parsers, reported OK by
both the install step and `doctor`. `runs` / `Runs` ask the tool to identify
itself instead. Use them for anything installed as a wrapper around a downloaded
binary, and prefer counting a step's output — `.so` files built — over trusting
the exit status of a command that logs its failure and exits 0 anyway.

`container.sh` pipes `git archive HEAD` into a clean `ubuntu:26.04` with a
sudo-capable non-root user, runs the bootstrap, and then checks that a second run
downloads nothing, that no tracked file was modified, that zsh starts silently,
and that `doctor` passes. Only tracked files go in, so nothing untracked in
`~/.config` — the gh token, shell history — reaches the container.

## WSL

- Windows terminals do not read Linux-side fonts. Cica has to be installed on
  the Windows side too; `bs.sh` prints where the TTFs are and where to copy them.
- The `/mnt/*` PATH pruning belongs to `zsh/.zshenv`, not here. `doctor` reports
  how many entries survive so a regression of that ~80ms saving is visible.
  Setting `appendWindowsPath=false` in `/etc/wsl.conf` would make the pruning
  unnecessary, but also removes `code`, `clip.exe`, `explorer.exe` and `winget`,
  which the keep-list in `.zshenv` preserves on purpose.
