#!/usr/bin/env bash
# =============================================================================
# bootstrap/bs.sh
# Reproduce this ~/.config on an Ubuntu machine, and keep its pins current.
#
#   bs.sh                 install everything (idempotent; safe to re-run)
#   bs.sh update          bump every pinned version, then review the git diff
#   bs.sh doctor          report where the machine differs from the manifests
#   bs.sh list            list the steps
#
# What goes where:
#   tools.tsv        prebuilt binaries from GitHub releases -> ~/.local/bin
#   apt.tsv          apt package set, by group
#   runtimes.tsv     nvim / node / pnpm / go / rust, each via its own manager
#   npm-globals.tsv  npm/pnpm global CLIs, version-independent of nvm's node
#   steps/*.sh       one file per concern, registering steps in numeric order
#
# Adding a GitHub-release tool is one line in tools.tsv; a global npm/pnpm CLI
# is one line in npm-globals.tsv. Nothing else.
# =============================================================================

# Not -e: a failing step is collected and reported at the end rather than
# aborting the run, so one broken download does not cost you the whole install.
# Each step body does run under -e, inside its own subshell (see dispatch).
set -uo pipefail

BOOTSTRAP_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$BOOTSTRAP_DIR/lib.sh"

# ----------------------------------------------------------------------------
# Step registry
# ----------------------------------------------------------------------------
declare -a STEP_IDS=()
declare -A STEP_DESC=() STEP_ROOT=() STEP_RUN=()

# register <id> <needs_root: yes|no> <description> <run_fn>
# Verification lives in bootstrap/tool, not next to the step -- see the doctor
# section below for why.
register() {
  STEP_IDS+=("$1")
  STEP_ROOT["$1"]="$2"
  STEP_DESC["$1"]="$3"
  STEP_RUN["$1"]="$4"
}

load_steps() {
  local f
  for f in "$BOOTSTRAP_DIR"/steps/*.sh; do
    [ -r "$f" ] || continue
    # shellcheck source=/dev/null
    . "$f"
  done
}

# ----------------------------------------------------------------------------
# Selection
# ----------------------------------------------------------------------------
ONLY="" SKIP_LIST="" DRY_RUN=0 NO_SUDO=0 ARCH_OVERRIDE=""

# --only/--skip match on a prefix, so `--only tool:` selects every tool and
# `--only tool:eza` selects one.
_matches() {
  local id="$1" list="$2" p
  local -a pat=()
  IFS=, read -r -a pat <<<"$list"
  for p in "${pat[@]}"; do
    [ -n "$p" ] || continue
    case "$id" in "$p"*) return 0 ;; esac
  done
  return 1
}

selected() {
  local id="$1"
  [ -n "$SKIP_LIST" ] && _matches "$id" "$SKIP_LIST" && return 1
  [ -z "$ONLY" ] && return 0
  _matches "$id" "$ONLY"
}

# ----------------------------------------------------------------------------
# Root access
# ----------------------------------------------------------------------------
SUDO_OK=0 SUDO_CMD=""
_SUDO_KEEPALIVE_PID=""

sudo_init() {
  # Running the whole script through sudo would install into /root/.local and
  # chown half of $HOME to root. Refuse instead of producing a broken machine.
  if [ "$(id -u)" = 0 ] && [ -n "${SUDO_USER:-}" ]; then
    die "do not run this with sudo -- run it as your own user; individual commands elevate themselves"
  fi

  if [ "$NO_SUDO" = 1 ]; then
    SUDO_OK=0
    return 0
  fi
  if [ "$(id -u)" = 0 ]; then
    SUDO_OK=1 SUDO_CMD=""
    return 0
  fi
  has sudo || { SUDO_OK=0; return 0; }

  # A dry run changes nothing, so do not make the user authenticate just to see
  # the plan -- and do show them the root steps rather than skipping them.
  if [ "$DRY_RUN" = 1 ]; then
    SUDO_OK=1 SUDO_CMD="sudo"
    return 0
  fi

  # shellcheck disable=SC2034  # SUDO_CMD is read by run_sudo in lib.sh
  # One prompt for the whole run, then keep the timestamp warm so a long apt
  # step cannot be interrupted by a second password prompt.
  if sudo -n true 2>/dev/null || { [ -t 0 ] && sudo -v; }; then
    SUDO_OK=1 SUDO_CMD="sudo"
    while true; do sudo -n true 2>/dev/null || break; sleep 50; done &
    _SUDO_KEEPALIVE_PID=$!
  else
    SUDO_OK=0
  fi
}

on_exit() {
  [ -n "$_SUDO_KEEPALIVE_PID" ] && kill "$_SUDO_KEEPALIVE_PID" 2>/dev/null
  tmp_cleanup
}
trap on_exit EXIT

# ----------------------------------------------------------------------------
# install
# ----------------------------------------------------------------------------
cmd_install() {
  local id rc ran=0
  local -a failed=() skipped=() rootless=()

  tmp_init || die "could not create the run's temporary directory"
  run mkdir -p "$LOCAL_BIN" "$FONT_DIR"

  for id in "${STEP_IDS[@]}"; do
    selected "$id" || continue
    ran=$((ran + 1))
    if [ "${STEP_ROOT[$id]}" = yes ] && [ "$SUDO_OK" != 1 ]; then
      rootless+=("$id")
      skip "$id: needs root"
      continue
    fi

    # Before each step, not just once at startup: steps run in subshells, so a
    # directory created by one (bob's nvim-bin, $PNPM_HOME, ~/.cargo/bin) is
    # invisible to the next unless the parent re-scans. Without this the plugin
    # restore silently skipped on a fresh machine, because `nvim` had just been
    # installed into a directory that was not on PATH when the run began.
    path_refresh
    head_ "$id  ${_C_DIM}${STEP_DESC[$id]}${_C_OFF}"
    # Subshell so the step body can use `set -e` (a failure inside it stops that
    # step immediately) without ending the whole run.
    # Unquoted on purpose: a registered command may carry arguments, as in
    # "tool_install eza", and must be word-split.
    # shellcheck disable=SC2086
    ( set -euo pipefail; ${STEP_RUN[$id]} )
    rc=$?
    case "$rc" in
      0)           ok "$id" ;;
      "$SKIP_RC")  skipped+=("$id") ;;
      *)           err "$id failed (exit $rc)"; failed+=("$id") ;;
    esac
  done

  printf '\n'
  head_ "Summary"
  # A typo in --only would otherwise select nothing, do nothing, and exit 0 --
  # which reads exactly like success.
  if [ "$ran" = 0 ]; then
    err "no step matched --only '$ONLY'. See: bs.sh list"
    return 2
  fi
  if [ ${#skipped[@]} -gt 0 ]; then skip "skipped: ${skipped[*]}"; fi
  if [ ${#rootless[@]} -gt 0 ]; then
    local list; list=$(IFS=,; printf '%s' "${rootless[*]}")
    warn "skipped ${#rootless[@]} step(s) needing root. When you have sudo:"
    warn "    $BOOTSTRAP_DIR/bs.sh --only $list"
  fi
  if [ ${#failed[@]} -gt 0 ]; then
    err "failed: ${failed[*]}"
    err "re-run just those with: bs.sh --only $(IFS=,; printf '%s' "${failed[*]}")"
    return 1
  fi
  ok "no failures"
  next_steps
}

next_steps() {
  cat <<EOF

--- Next steps --------------------------------------------------------------
  * Start a new shell: exec zsh
  * Check the result:  $BOOTSTRAP_DIR/bs.sh doctor
EOF
  # `usermod -aG` updates the account database, but a child shell inherits the
  # current process's supplementary groups. `exec zsh` alone therefore cannot
  # activate Docker access; make that easy to notice after the first install.
  if id -nG "$(id -un)" 2>/dev/null | tr ' ' '\n' | grep -qx docker \
      && ! id -nG | tr ' ' '\n' | grep -qx docker; then
    cat <<'EOF'
  * Docker: run `newgrp docker`, or log out and back in, before using it
    without sudo. Restart WSL if a normal logout does not refresh the group.
EOF
  fi
  if is_wsl; then
    cat <<'EOF'
  * WSL: Windows terminals do not read Linux-side fonts. Install Cica on the
    Windows side too and select it as the terminal font.
EOF
    if has wslpath && has wslvar; then
      local up
      up=$(wslpath "$(wslvar USERPROFILE 2>/dev/null)" 2>/dev/null) || up=""
      [ -n "$up" ] && printf '      TTFs are in %s -- copy them to %s/Downloads and double-click.\n' \
        "$FONT_DIR" "$up"
    fi
  fi
  printf -- '-----------------------------------------------------------------------------\n'
}

# ----------------------------------------------------------------------------
# list
# ----------------------------------------------------------------------------
cmd_list() {
  printf '%-22s %-6s %s\n' ID ROOT DESCRIPTION
  printf '%-22s %-6s %s\n' '---' '----' '-----------'
  local id
  for id in "${STEP_IDS[@]}"; do
    printf '%-22s %-6s %s\n' "$id" "${STEP_ROOT[$id]}" "${STEP_DESC[$id]}"
  done
}

# ----------------------------------------------------------------------------
# doctor / update -- delegated to bootstrap/tool
# ----------------------------------------------------------------------------
# These two are the parts with real logic: HTTP, JSON and version algebra, which
# is also where the shell version had its actual bugs (0.9.3 comparing above
# 0.10.0; tags with and without a v prefix). They live in Go so `go test` can pin
# that behaviour down.
#
# They are deliberately kept out of the install path: a bare machine has no Go
# until `bs.sh` has installed it, and neither doctor nor update is needed before
# then -- nothing can be behind upstream on day one.
#
# The binary is built on first use and cached outside the repository. It is never
# committed: it would be per-architecture, impossible to verify by reading, and
# stale the moment a .go file changed.
BSTOOL="${XDG_CACHE_HOME:-$HOME/.cache}/dotconfig/bstool"

bstool() {
  local src="$BOOTSTRAP_DIR/tool"
  if [ ! -x "$BSTOOL" ] || [ -n "$(find "$src" -name '*.go' -newer "$BSTOOL" -print -quit 2>/dev/null)" ]; then
    if ! has go; then
      err "doctor and update need Go, which bs.sh itself installs."
      err "Run: $BOOTSTRAP_DIR/bs.sh --only go"
      return 1
    fi
    # Keep stdout clean for `doctor --json`; build progress is diagnostic.
    info "building the doctor/update tool" >&2
    mkdir -p "${BSTOOL%/*}"
    ( cd "$src" && go build -o "$BSTOOL" . ) || { err "failed to build $src"; return 1; }
  fi
  BOOTSTRAP_DIR="$BOOTSTRAP_DIR" CONFIG_DIR="$CONFIG_DIR" "$BSTOOL" "$@"
}

# The Go tool checks its own PATH list against zsh's $path. Print it from here so
# lib.sh stays the only place that list is written down.
print_path_contract() {
  printf '%s\n' "${PATH_WANT[@]}"
}

# ----------------------------------------------------------------------------
# CLI
# ----------------------------------------------------------------------------
usage() {
  cat <<'EOF'
Usage: bs.sh [COMMAND] [OPTIONS]

Commands
  install        (default) run every step, in order. Idempotent.
  update        bump every pinned version in the manifests
  doctor        report where this machine differs from the manifests
  list          list the steps

Options
  --only ID,…    run only these steps (prefix match: --only tool:)
  --skip ID,…    skip these steps
  --dry-run      print what would change, change nothing
  --no-sudo      skip the steps that need root
  --arch ARCH    override architecture detection (testing)

  doctor and update take their own options; try `bs.sh update --help`.
  -h, --help     this text

Examples
  bs.sh                       set up or repair the machine
  bs.sh --only tool:          reinstall the pinned binaries
  bs.sh update --check        is anything behind upstream?
  bs.sh doctor
EOF
}

main() {
  local cmd=install
  case "${1:-}" in
    install|update|doctor|list) cmd=$1; shift ;;
    # Undocumented, for the Go tool: see print_path_contract above.
    --print-path-contract) path_init; print_path_contract; exit 0 ;;
    -h|--help) usage; exit 0 ;;
  esac

  # doctor and update are the Go tool's; hand the arguments straight over rather
  # than parsing them twice with two notions of what is valid. path_init first:
  # the tool resolves binaries with the PATH it inherits, so without it doctor
  # reports tools missing that a real shell would find.
  if [ "$cmd" = doctor ] || [ "$cmd" = update ]; then
    path_init
    bstool "$cmd" "$@"
    exit $?
  fi

  while [ $# -gt 0 ]; do
    case "$1" in
      --only)     ONLY="${2:?--only needs a value}"; shift 2 ;;
      --only=*)   ONLY="${1#*=}"; shift ;;
      --skip)     SKIP_LIST="${2:?--skip needs a value}"; shift 2 ;;
      --skip=*)   SKIP_LIST="${1#*=}"; shift ;;
      --arch)     ARCH_OVERRIDE="${2:?--arch needs a value}"; shift 2 ;;
      --arch=*)   ARCH_OVERRIDE="${1#*=}"; shift ;;
      --dry-run)  DRY_RUN=1; shift ;;
      --no-sudo)  NO_SUDO=1; shift ;;
      -h|--help)  usage; exit 0 ;;
      -*)         err "unknown option: $1"; usage; exit 2 ;;
      # A bare word here is most likely the old `bs.sh install_eza` convention,
      # which --only replaced.
      *)          err "unexpected argument: $1 (did you mean --only $1 ?)"; exit 2 ;;
    esac
  done

  export ARCH_OVERRIDE DRY_RUN
  arch_init
  arch_ok || warn "unrecognised architecture $ARCH_RAW: steps needing prebuilt binaries will skip"
  path_init
  load_steps

  case "$cmd" in
    install) sudo_init; cmd_install ;;
    update)  cmd_update ;;
    doctor)  cmd_doctor ;;
    list)    cmd_list ;;
  esac
}

main "$@"
