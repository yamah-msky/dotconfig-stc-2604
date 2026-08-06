# shellcheck shell=bash
# Language runtimes, each through its own version manager, pinned by runtimes.tsv.
# Unlike tools.tsv these are not "download one binary", which is why they are
# hand-written steps rather than manifest-driven.

rt_ref() { manifest_field runtimes.tsv "$1" 2; }
rt_min() { manifest_field runtimes.tsv "$1" 3; }

# ----------------------------------------------------------------------------
# Rust
# ----------------------------------------------------------------------------
# Tracks stable on purpose (see the note in runtimes.tsv): nothing here depends
# on a rustc version, and re-downloading a ~200MB toolchain per bump is not worth
# it. --no-modify-path because zsh/.zshenv already has ~/.cargo/bin.
_rustup_install() {
  curl "${CURL_OPTS[@]}" https://sh.rustup.rs | sh -s -- -y --no-modify-path --profile default
}

step_rust() {
  local min; min=$(rt_min rust)
  if has cargo; then
    local cur; cur=$(version_of cargo)
    if ver_ge "$cur" "$min"; then
      ok "rust $cur (>= $min)"
      return 0
    fi
    info "rust $cur is older than $min; updating"
    run rustup update stable
    return
  fi
  info "installing rustup"
  run _rustup_install
  # shellcheck disable=SC1091
  [ "${DRY_RUN:-0}" = 1 ] || . "$HOME/.cargo/env"

  # rustfmt and clippy back conform.nvim and rust_analyzer's checkOnSave.
  run rustup component add rustfmt clippy
}

# ----------------------------------------------------------------------------
# Go
# ----------------------------------------------------------------------------
# Ubuntu's `golang-go` on 26.04 is current (1.26.x), unlike noble's EOL 1.22.2 --
# but its exact patch is Ubuntu's to pick, not this repo's, so the pin still
# comes from the upstream tarball, shimmed into ~/.local/bin (which precedes
# /usr/bin, so an apt copy is shadowed rather than fought with). step_go reuses
# an apt copy instead of downloading only when it happens to exactly match the
# pin -- see the apt_go loop below.
step_go() {
  local ref; ref=$(rt_ref go)
  arch_require

  if [ -x "$GO_ROOT/bin/go" ] && [ "$("$GO_ROOT/bin/go" version | awk '{print $3}')" = "go$ref" ]; then
    ok "go $ref already in $GO_ROOT"
    run ln -sf "$GO_ROOT/bin/go" "$LOCAL_BIN/go"
    run ln -sf "$GO_ROOT/bin/gofmt" "$LOCAL_BIN/gofmt"
    return 0
  fi

  # apt's Go is current, not EOL, on 26.04 -- but its patch version is not
  # something this repo controls, so it is only reused when it already happens
  # to exactly match the pin. Never apt-installed here: doing so would usually
  # still miss the exact pin and end up downloading the tarball anyway.
  local apt_go
  for apt_go in /usr/lib/go-*/bin/go; do
    [ -x "$apt_go" ] || continue
    if [ "$("$apt_go" version | awk '{print $3}')" = "go$ref" ]; then
      ok "go $ref already at $apt_go (apt); linking instead of downloading"
      run ln -sf "$apt_go" "$LOCAL_BIN/go"
      run ln -sf "$(dirname "$apt_go")/gofmt" "$LOCAL_BIN/gofmt"
      return 0
    fi
  done

  has go && info "go $(version_of go) at $(command -v go); installing the pinned $ref"

  local url tmp
  url="https://go.dev/dl/go${ref}.linux-${GOARCH}.tar.gz"
  info "$url"
  tmp=$(mktmp)
  download "$url" "$tmp/go.tar.gz"
  extract "$tmp/go.tar.gz" "$tmp"
  if [ "${DRY_RUN:-0}" = 1 ]; then
    dry "mv $tmp/go -> $GO_ROOT; symlink go and gofmt into $LOCAL_BIN"
    return 0
  fi
  rm -rf "$GO_ROOT"
  mkdir -p "$(dirname "$GO_ROOT")"
  mv "$tmp/go" "$GO_ROOT"
  ln -sf "$GO_ROOT/bin/go" "$LOCAL_BIN/go"
  ln -sf "$GO_ROOT/bin/gofmt" "$LOCAL_BIN/gofmt"
  hash -r 2>/dev/null || true
  info "go is now $(version_of go)"
}

# ----------------------------------------------------------------------------
# Node, via nvm
# ----------------------------------------------------------------------------
# nvm rather than a plain tarball because zsh/.zshenv deliberately does not
# source nvm.sh (~620ms per shell): it globs $NVM_DIR/versions/node/*/bin and
# reads $NVM_DIR/alias/default instead. So the alias has to be set, or that glob
# silently picks a different version than the pin.
step_node() {
  local ref; ref=$(rt_ref node)

  if [ ! -s "$NVM_DIR/nvm.sh" ]; then
    info "cloning nvm into $NVM_DIR"
    if [ -d "$NVM_DIR/.git" ]; then
      run git -C "$NVM_DIR" fetch --tags -q origin
    else
      run git clone -q https://github.com/nvm-sh/nvm.git "$NVM_DIR"
    fi
    if [ "${DRY_RUN:-0}" != 1 ]; then
      local tag; tag=$(git -C "$NVM_DIR" describe --abbrev=0 --tags 2>/dev/null)
      [ -n "$tag" ] && git -C "$NVM_DIR" checkout -q "$tag"
    fi
  fi

  if [ "${DRY_RUN:-0}" = 1 ]; then
    dry "nvm install $ref && nvm alias default $ref"
    return 0
  fi

  if [ -x "$NVM_DIR/versions/node/v$ref/bin/node" ] \
     && [ "$(cat "$NVM_DIR/alias/default" 2>/dev/null)" = "$ref" ]; then
    ok "node v$ref installed and set as default"
    return 0
  fi

  export NVM_DIR
  # nvm.sh is not shellcheck-clean and is not ours.
  # shellcheck disable=SC1091
  . "$NVM_DIR/nvm.sh"
  nvm install "$ref" || return 1
  # The alias must be the exact version, not "lts/*": .zshenv reads this file
  # literally and globs versions/node/v<it>*.
  nvm alias default "$ref" >/dev/null || return 1
  info "node $(node -v), default alias -> $(cat "$NVM_DIR/alias/default")"
}

# ----------------------------------------------------------------------------
# pnpm, and the editor's node tools
# ----------------------------------------------------------------------------
_pnpm_install() {
  # PNPM_VERSION is what keeps the installer from ignoring the pin and taking
  # whatever is newest -- which it did, landing 11.18.0 against a pinned 11.15.0.
  curl "${CURL_OPTS[@]}" https://get.pnpm.io/install.sh \
    | env SHELL="$(command -v bash)" PNPM_HOME="$PNPM_HOME" \
          PNPM_VERSION="$(rt_ref pnpm)" sh -
}

step_pnpm() {
  local ref min; ref=$(rt_ref pnpm); min=$(rt_min pnpm)
  if has pnpm && ! needs_install pnpm "$ref" "$min"; then
    ok "pnpm $(version_of pnpm)"
  elif has corepack; then
    # corepack ships with node, so this needs no network install script and it
    # takes an exact version. But `install --global` only *registers* the version:
    # the shim comes from `enable`, and without it pnpm is nowhere on PATH.
    info "installing pnpm@$ref via corepack"
    run corepack install --global "pnpm@$ref"
    # Into $PNPM_HOME/bin rather than next to corepack, which lives inside one
    # node version's directory -- the same trap that would have taken prettier
    # and eslint_d with it on the next node bump.
    run mkdir -p "$PNPM_HOME/bin"
    run corepack enable pnpm --install-directory "$PNPM_HOME/bin"
    path_refresh
  else
    info "installing pnpm"
    run _pnpm_install
    # The installer creates $PNPM_HOME, which therefore was not on PATH when the
    # run started -- without this, the node-tools step below cannot see pnpm.
    path_refresh
  fi

  # Fail rather than carry on: the node tools below need pnpm, and reporting this
  # step as OK while they silently went missing is how the container run ended up
  # without prettier or eslint_d.
  if [ "${DRY_RUN:-0}" != 1 ] && ! has pnpm; then
    err "pnpm is still not on PATH after installing it"
    return 1
  fi

  # prettier and eslint_d are what conform.nvim and nvim-lint shell out to. On
  # this machine they were installed with `npm -g`, which puts them inside a
  # single node version's directory -- so bumping node made them vanish.
  # $PNPM_HOME/bin is version-independent and on PATH via zsh/.zshenv.
  step_node_globals
}

# prettier   conform.nvim's formatter for web files
# eslint_d   nvim-lint's linter
# tree-sitter-cli  nvim-treesitter's rewritten branch shells out to `tree-sitter
#                  build` for every parser. Without it each build dies with
#                  ENOENT and you get an editor with no syntax highlighting --
#                  which is exactly what the container produced before this.
NODE_GLOBALS=(prettier eslint_d tree-sitter-cli)

# The npm package is tree-sitter-cli; the binary it installs is tree-sitter.
node_global_bin() {
  case "$1" in
    tree-sitter-cli) printf 'tree-sitter' ;;
    *) printf '%s' "$1" ;;
  esac
}

# pnpm 10 stopped running dependency build scripts unless the package is
# approved, and tree-sitter-cli's npm package is only a JS wrapper: its
# postinstall downloads the native binary the wrapper spawns. Blocked, the
# install still "succeeds" and leaves a shim on PATH that dies with ENOENT on
# every call -- so `pnpm add -g tree-sitter-cli` alone produced an editor with
# no parsers at all. --allow-build names the exception explicitly rather than
# turning build scripts back on globally.
PNPM_ALLOW_BUILD=(--allow-build=tree-sitter-cli)

# Remove any `npm -g` copy of a tool we manage with pnpm. zsh/.zshenv puts nvm's
# node bin ahead of $PNPM_HOME/bin on purpose -- that is how it gets the default
# node without sourcing nvm.sh -- so a leftover npm -g copy keeps winning in the
# shell you actually use, and this step would reinstall on every single run.
node_globals_deduplicate() {
  has npm || return 0
  # Test the directory rather than asking npm: `npm ls -g --parseable <pkg>`
  # exits 0 with empty output for a package that is not installed, so using it
  # made this run `npm uninstall` on every invocation.
  local root; root=$(npm root -g 2>/dev/null) || return 0
  [ -d "$root" ] || return 0
  local g dupes=()
  for g in "${NODE_GLOBALS[@]}"; do
    [ -d "$root/$g" ] && dupes+=("$g")
  done
  [ ${#dupes[@]} -gt 0 ] || return 0
  info "removing the npm -g copies of ${dupes[*]} (pnpm owns these now)"
  run npm uninstall -g "${dupes[@]}" >/dev/null 2>&1 || warn "npm uninstall -g reported a problem"
}

step_node_globals() {
  # A dry run installed no pnpm, so there is nothing to check and nothing to do.
  # Say what would happen instead of failing on an absence we created.
  if [ "${DRY_RUN:-0}" = 1 ]; then
    dry "pnpm add -g ${PNPM_ALLOW_BUILD[*]} ${NODE_GLOBALS[*]}   # whichever are missing, broken or under \$NVM_DIR"
    return 0
  fi
  has pnpm || { err "pnpm missing; cannot install the editor's node tools"; return 1; }
  local want=() g bin where
  for g in "${NODE_GLOBALS[@]}"; do
    bin=$(node_global_bin "$g")
    if ! has "$bin"; then
      want+=("$g")
      continue
    fi
    # On PATH but not runnable: an earlier install whose postinstall was blocked.
    # Reinstalling with --allow-build is what repairs it.
    if ! runs "$bin"; then
      info "$bin is on PATH but does not run; reinstalling it"
      want+=("$g")
      continue
    fi
    # Present but installed inside one node version's directory: it works today
    # and disappears the next time node is bumped, so move it.
    where=$(command -v "$bin")
    case "$where" in
      "$NVM_DIR"/*) info "$bin lives in $where; reinstalling it version-independently"; want+=("$g") ;;
    esac
  done
  if [ ${#want[@]} -eq 0 ]; then
    ok "editor node tools present: ${NODE_GLOBALS[*]}"
  fi
  if [ ${#want[@]} -gt 0 ]; then
    info "installing ${want[*]} into \$PNPM_HOME"
    run pnpm add -g "${PNPM_ALLOW_BUILD[@]}" "${want[@]}"
  fi
  node_globals_deduplicate
  path_refresh

  # Verify the outcome rather than assume it: the whole point is that these end
  # up somewhere a node bump cannot take them.
  [ "${DRY_RUN:-0}" = 1 ] && return 0
  local bad=""
  for g in "${NODE_GLOBALS[@]}"; do
    bin=$(node_global_bin "$g")
    has "$bin" || { bad+=" $bin(missing)"; continue; }
    runs "$bin" || { bad+=" $bin(does not run)"; continue; }
    case "$(command -v "$bin")" in "$NVM_DIR"/*) bad+=" $bin(still under nvm)" ;; esac
  done
  if [ -n "$bad" ]; then
    warn "editor node tools not usable and version-independent:$bad"
  else
    ok "editor node tools are version-independent"
  fi
}

register rust  no "Rust toolchain via rustup (tracks stable)"        step_rust
register go    no "Go from the pinned upstream tarball"              step_go
register node  no "Node via nvm, with the default alias pinned"      step_node
register pnpm  no "pnpm, plus prettier/eslint_d for the editor"      step_pnpm