# shellcheck shell=bash
# =============================================================================
# bootstrap/lib.sh
# Helpers shared by bs.sh and every steps/*.sh. Sourced, never executed.
# =============================================================================

# ----------------------------------------------------------------------------
# Logging
# ----------------------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  _C_HEAD=$'\033[1;36m'; _C_INFO=$'\033[1;34m'; _C_OK=$'\033[1;32m'
  _C_WARN=$'\033[1;33m'; _C_ERR=$'\033[1;31m';  _C_DIM=$'\033[2m'; _C_OFF=$'\033[0m'
else
  _C_HEAD='' _C_INFO='' _C_OK='' _C_WARN='' _C_ERR='' _C_DIM='' _C_OFF=''
fi

head_()  { printf '\n%s==>%s %s\n' "$_C_HEAD" "$_C_OFF" "$*"; }
info()   { printf '%s  ->%s %s\n'  "$_C_INFO" "$_C_OFF" "$*"; }
ok()     { printf '%s  OK%s %s\n'  "$_C_OK"   "$_C_OFF" "$*"; }
skip()   { printf '%s  --%s %s\n'  "$_C_DIM"  "$_C_OFF" "$*"; }
warn()   { printf '%s  !!%s %s\n'  "$_C_WARN" "$_C_OFF" "$*" >&2; }
err()    { printf '%s  XX%s %s\n'  "$_C_ERR"  "$_C_OFF" "$*" >&2; }
dry()    { printf '%s  ..%s %s\n'  "$_C_DIM"  "$_C_OFF" "$*"; }
die()    { err "$*"; exit 1; }

# ----------------------------------------------------------------------------
# Paths and layout
# ----------------------------------------------------------------------------
: "${XDG_CONFIG_HOME:=$HOME/.config}"
: "${XDG_CACHE_HOME:=$HOME/.cache}"
: "${XDG_DATA_HOME:=$HOME/.local/share}"

# The layout constants every step reads. shellcheck only sees this file, not the
# steps/*.sh that consume them, so the "unused" warnings here are wrong.
BOOTSTRAP_DIR="${BOOTSTRAP_DIR:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)}"
# shellcheck disable=SC2034
CONFIG_DIR="$(dirname -- "$BOOTSTRAP_DIR")"
LOCAL_BIN="$HOME/.local/bin"
# shellcheck disable=SC2034
FONT_DIR="$XDG_DATA_HOME/fonts"
GO_ROOT="$HOME/.local/go"
NVM_DIR="${NVM_DIR:-$XDG_CONFIG_HOME/nvm}"
PNPM_HOME="${PNPM_HOME:-$XDG_DATA_HOME/pnpm}"

# Exit code meaning "this step declined to run", as opposed to failing. 78 is
# EX_CONFIG from sysexits.h; anything outside 0/78 is a real failure.
SKIP_RC=78
step_skip() { skip "$*"; return "$SKIP_RC"; }

# Mirror of the `path=(...)` list in zsh/.zshenv, in the same order. It has to be
# duplicated because that file is zsh (typeset -U, (N-/) globbing) and bash
# cannot read it; `bs.sh doctor` diffs the two so drift is visible rather than
# theoretical. Nonexistent directories are dropped, exactly like the (N-/) filter.
declare -a PATH_WANT=()

path_init() {
  local d
  local -a want=(
    "$LOCAL_BIN"
    "$HOME/.cargo/bin"
    "$XDG_DATA_HOME/bob/nvim-bin"
    "$HOME/.pixi/bin"
    "$PNPM_HOME/bin"
    "$PNPM_HOME"
    "${GOPATH:-$HOME/go}/bin"
    "$GO_ROOT/bin"
  )
  # nvm's default node, the same way .zshenv finds it: read alias/default and
  # glob for it, rather than sourcing nvm.sh.
  local want_node=""
  [ -r "$NVM_DIR/alias/default" ] && want_node=$(<"$NVM_DIR/alias/default")
  local -a nodebin=()
  if [ -n "$want_node" ]; then
    nodebin=("$NVM_DIR/versions/node/v${want_node#v}"*/bin)
  fi
  [ -d "${nodebin[0]:-}" ] || nodebin=("$NVM_DIR"/versions/node/*/bin)
  # Appended last, so the loop below prepends it last and it ends up FIRST on
  # PATH -- matching .zshenv, which puts nvm's bin ahead of everything else. The
  # order has to agree or `doctor` reports a different binary than the one your
  # shell would actually run.
  [ -d "${nodebin[-1]:-}" ] && want+=("${nodebin[-1]}")

  # Read by bs.sh's doctor to check this list against zsh's own $path.
  # shellcheck disable=SC2034
  PATH_WANT=("${want[@]}")
  for d in "${want[@]}"; do
    case ":$PATH:" in
      *":$d:"*) ;;
      *) [ -d "$d" ] && PATH="$d:$PATH" ;;
    esac
  done
  export PATH
}

# Re-run the PATH scan after a step creates a directory that did not exist when
# the run started -- bob's nvim-bin and $PNPM_HOME are both created by the very
# step that then needs to invoke what it just installed. Only ever adds, so
# calling it repeatedly is free.
path_refresh() {
  path_init
  hash -r 2>/dev/null || true
}

# ----------------------------------------------------------------------------
# Predicates
# ----------------------------------------------------------------------------
has() { command -v "$1" >/dev/null 2>&1; }
need() { has "$1" || die "required command not found: $1"; }

# Presence on PATH is not health. A tool installed as a wrapper script -- npm
# packages that fetch a native binary in a postinstall are the case that bit us
# -- is a real, executable file whether or not the binary behind it ever
# arrived, so `has` reported working tools that died on every call. Ask the tool
# to identify itself instead. Only use this on tools whose --version is cheap and
# side-effect free; it runs on every bootstrap.
runs() { has "$1" && "$1" --version >/dev/null 2>&1; }

is_wsl() { grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null; }

# ----------------------------------------------------------------------------
# Mutation gate
# ----------------------------------------------------------------------------
# Everything that changes the system goes through run() so --dry-run can print it
# instead. Pipelines (curl | sh) must be wrapped in a named function and passed
# here as `run install_foo` -- that keeps the dry-run output honest and one line.
run() {
  if [ "${DRY_RUN:-0}" = 1 ]; then dry "$*"; return 0; fi
  "$@"
}

# Run one command as root. bs.sh sets both variables once:
#   SUDO_OK=1  root actions are possible at all
#   SUDO_CMD   "sudo", or empty when we already are root
# Steps registered as needing root are filtered out before they run, so reaching
# here with SUDO_OK=0 means a step was registered wrongly.
run_sudo() {
  [ "${SUDO_OK:-0}" = 1 ] || { warn "needs root: $*"; return "$SKIP_RC"; }
  if [ -n "${SUDO_CMD:-}" ]; then run "$SUDO_CMD" "$@"; else run "$@"; fi
}

# ----------------------------------------------------------------------------
# Temporary directories
# ----------------------------------------------------------------------------
RUN_TMP_ROOT=""

# Steps execute in subshells and mktmp is normally called through command
# substitution.  An in-memory array therefore cannot communicate temporary
# directories back to the parent process.  Keep one exported, run-scoped root
# instead; removing it also removes every child a step created.
tmp_init() {
  [ "${DRY_RUN:-0}" = 1 ] && return 0
  RUN_TMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/dotconfig-bootstrap.XXXXXX") || return 1
  export RUN_TMP_ROOT
}

mktmp() {
  if [ "${DRY_RUN:-0}" = 1 ]; then
    printf '%s\n' "${TMPDIR:-/tmp}/dotconfig-bootstrap.dry-run/step"
    return 0
  fi
  [ -n "$RUN_TMP_ROOT" ] || { warn "temporary root is not initialized"; return 1; }
  mktemp -d "$RUN_TMP_ROOT/step.XXXXXX"
}

tmp_cleanup() {
  [ -n "$RUN_TMP_ROOT" ] && rm -rf -- "$RUN_TMP_ROOT"
  return 0
}

# ----------------------------------------------------------------------------
# Architecture
# ----------------------------------------------------------------------------
# Release assets are named half a dozen different ways. Resolve every naming
# scheme once here so tools.tsv can stay declarative, and leave the variables
# empty on an arch we have no assets for -- arch_require() then turns that into
# an explicit SKIP instead of a 404 halfway through a download.
arch_init() {
  ARCH_RAW="${ARCH_OVERRIDE:-$(uname -m)}"
  case "$ARCH_RAW" in
    x86_64|amd64)
      ARCH=x86_64
      RUST_GNU=x86_64-unknown-linux-gnu
      RUST_MUSL=x86_64-unknown-linux-musl
      GOARCH=amd64
      DEB_ARCH=amd64
      BOB_ARCH=x86_64
      ;;
    aarch64|arm64)
      ARCH=aarch64
      RUST_GNU=aarch64-unknown-linux-gnu
      RUST_MUSL=aarch64-unknown-linux-musl
      GOARCH=arm64
      DEB_ARCH=arm64
      # bob names its ARM asset "arm", not "aarch64" -- and it is not documented
      # whether that build is 64-bit, so 50-runtimes prefers cargo there.
      BOB_ARCH=arm
      ;;
    *)
      ARCH='' RUST_GNU='' RUST_MUSL='' GOARCH='' DEB_ARCH='' BOB_ARCH=''
      ;;
  esac
}
arch_ok() { [ -n "${RUST_GNU:-}" ]; }
arch_require() { arch_ok || step_skip "no prebuilt assets for $ARCH_RAW"; }

# ----------------------------------------------------------------------------
# Versions
# ----------------------------------------------------------------------------
# First version-looking token of `<tool> --version`, with the handful of tools
# that do not follow that convention special-cased. Verified quirks:
#   eza      prints its version on line 2; line 1 has no digits at all
#   sheldon  prints its own version on line 1 and rustc's on line 2
#   go       "go version go1.22.2 linux/amd64" -- needs the `go` stripped
#   yt-dlp   a date, "2026.03.17", which still sorts correctly with sort -V
version_of() {
  local tool="$1" out=""
  case "$tool" in
    go)   out=$(go version 2>/dev/null) ;;
    node) out=$(node -v 2>/dev/null) ;;
    *)    out=$("$tool" --version 2>&1) ;;
  esac
  printf '%s' "$out" | grep -oE '[0-9]+(\.[0-9]+){1,2}' | head -n1
}

# ver_ge A B  <=>  A >= B. sort -V rather than a string compare, because
# 0.9.3 vs 0.10.0 is exactly the case a string compare gets backwards.
ver_ge() {
  [ "$1" = "$2" ] && return 0
  [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$2" ]
}

# The pinned tag with any leading `v` removed, for comparing against --version
# output. tools.tsv stores the tag verbatim because upstreams disagree about the
# prefix (sheldon and uv publish "0.8.5", everyone else "v0.8.5").
ref_version() { printf '%s' "${1#v}"; }

# needs_install <tool> <ref> <min>  ->  0 when work is required.
#
# An exact pin is compared with != rather than <: the goal is reproducibility,
# so a version newer than the pin is drift too and gets replaced. This is also
# the fix for the old `has $tool` guard, which let an old distro package satisfy
# the check and silently skip the install forever.
needs_install() {
  local tool="$1" ref="$2" min="$3" cur
  has "$tool" || return 0
  cur=$(version_of "$tool")
  [ -n "$cur" ] || return 0                 # unparseable: reinstall rather than guess
  if [ "$ref" != latest ]; then
    [ "$cur" != "$(ref_version "$ref")" ]
  else
    [ "$min" != - ] && ! ver_ge "$cur" "$min"
  fi
}

# ----------------------------------------------------------------------------
# Manifests
# ----------------------------------------------------------------------------
# Read a TSV, dropping comments and blank lines, and call back per row with the
# fields as positional arguments:  manifest_each <file> <callback>
manifest_each() {
  local file="$BOOTSTRAP_DIR/$1" cb="$2" line
  [ -r "$file" ] || die "manifest not found: $file"
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    local -a f=()
    IFS=$'\t' read -r -a f <<<"$line"
    [ ${#f[@]} -gt 0 ] || continue
    "$cb" "${f[@]}"
  done <"$file"
}

# One row by its first field:  manifest_row <file> <key>  -> tab-separated line
manifest_row() {
  local file="$BOOTSTRAP_DIR/$1" key="$2"
  awk -F'\t' -v k="$key" '!/^#/ && NF && $1 == k { print; exit }' "$file"
}

# Field N (1-based) of a row.
manifest_field() {
  local file="$1" key="$2" n="$3"
  manifest_row "$file" "$key" | cut -d$'\t' -f"$n"
}

# Expand the {PLACEHOLDER}s used by tools.tsv asset names.
expand_asset() {
  local s="$1" tag="$2"
  s=${s//\{TAG\}/$tag}
  s=${s//\{VERSION\}/$(ref_version "$tag")}
  s=${s//\{ARCH\}/${ARCH:-}}
  s=${s//\{RUST_GNU\}/${RUST_GNU:-}}
  s=${s//\{RUST_MUSL\}/${RUST_MUSL:-}}
  s=${s//\{GOARCH\}/${GOARCH:-}}
  s=${s//\{DEB_ARCH\}/${DEB_ARCH:-}}
  s=${s//\{BOB_ARCH\}/${BOB_ARCH:-}}
  printf '%s' "$s"
}

# ----------------------------------------------------------------------------
# Downloading
# ----------------------------------------------------------------------------
CURL_OPTS=(--proto '=https' --tlsv1.2 -fsSL --retry 3 --retry-delay 2 --retry-connrefused)

# The install path never calls the GitHub API: the URL is computable from the
# pinned tag in the manifest. That keeps runs deterministic and away from the
# 60 requests/hour anonymous rate limit. Only `bs.sh update` talks to the API.
release_url() {
  local repo="$1" tag="$2" asset="$3"
  if [ "$tag" = latest ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$repo" "$asset"
  else
    printf 'https://github.com/%s/releases/download/%s/%s\n' "$repo" "$tag" "$asset"
  fi
}

# Latest release tag. Used by `update` only. gh first (authenticated, 5000/h).
latest_tag() {
  local repo="$1" tag=""
  if has gh; then
    tag=$(gh api "repos/$repo/releases/latest" --jq .tag_name 2>/dev/null) || tag=""
  fi
  if [ -z "$tag" ] && has jq; then
    tag=$(curl "${CURL_OPTS[@]}" "https://api.github.com/repos/$repo/releases/latest" 2>/dev/null \
          | jq -r '.tag_name // empty') || tag=""
  fi
  if [ -z "$tag" ]; then
    tag=$(curl "${CURL_OPTS[@]}" "https://api.github.com/repos/$repo/releases/latest" 2>/dev/null \
          | grep -m1 '"tag_name"' | cut -d'"' -f4) || tag=""
  fi
  [ -n "$tag" ] && printf '%s\n' "$tag"
}

download() {
  local url="$1" dest="$2"
  run curl "${CURL_OPTS[@]}" "$url" -o "$dest"
}

# Verify against an upstream .sha256 sidecar when the project publishes one
# (starship, uv and pixi do). Absent sidecar is not an error -- most projects
# have none, and demanding one would mean pinning hashes in the manifest and
# re-downloading on every bump.
verify_sha256() {
  local file="$1" url="$2" expected=""
  [ "${DRY_RUN:-0}" = 1 ] && return 0
  expected=$(curl "${CURL_OPTS[@]}" "$url.sha256" 2>/dev/null | tr -d '\n' | awk '{print $1}') || expected=""
  [ -n "$expected" ] || return 0
  local got
  got=$(sha256sum "$file" | awk '{print $1}')
  if [ "$got" != "$expected" ]; then
    err "checksum mismatch for $(basename "$file")"
    err "  expected $expected"
    err "  got      $got"
    return 1
  fi
  info "checksum ok"
}

extract() {
  local archive="$1" dest="$2"
  case "$archive" in
    *.tar.gz|*.tgz) run tar -xzf "$archive" -C "$dest" ;;
    *.tar.xz|*.txz) run tar -xJf "$archive" -C "$dest" ;;
    *.zip)          run unzip -q -o "$archive" -d "$dest" ;;
    *)              warn "unknown archive type: $archive"; return 1 ;;
  esac
}

# Find each named binary anywhere under a directory and install it to ~/.local/bin.
install_bins() {
  local src="$1"; shift
  local name found
  for name in "$@"; do
    if [ "${DRY_RUN:-0}" = 1 ]; then
      dry "install $name -> $LOCAL_BIN/$name"
      continue
    fi
    found=$(find "$src" -type f -name "$name" -print -quit 2>/dev/null)
    if [ -z "$found" ]; then
      warn "$name not found in archive"
      return 1
    fi
    install -m 0755 "$found" "$LOCAL_BIN/$name" || return 1
    info "$LOCAL_BIN/$name"
  done
}
