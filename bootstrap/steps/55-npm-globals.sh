# shellcheck shell=bash
# npm-globals.tsv: user-declared CLIs installed globally via pnpm, into
# $PNPM_HOME rather than inside one node version's directory -- the same fix
# NODE_GLOBALS applies in steps/50-runtimes.sh for prettier/eslint_d/
# tree-sitter-cli, extended to whatever the manifest lists. See the header of
# npm-globals.tsv for why this matters.
#
# One step is registered per row, mirroring tools.tsv/tool_install in
# steps/40-tools.sh: adding a CLI means adding one line and nothing else.
#
# Runs after step_pnpm (steps/50-runtimes.sh, file 50 < this file's 55), which
# is what creates $PNPM_HOME.

npm_global_install() {
  local name="$1" row package ref bin
  row=$(manifest_row npm-globals.tsv "$name") || true
  [ -n "$row" ] || die "npm-globals.tsv: no row named $name"
  IFS=$'\t' read -r name package ref bin <<<"$row"
  [ -n "$bin" ] || bin="$name"

  has pnpm || step_skip "pnpm is not installed (run: bs.sh --only pnpm)"

  if [ "${DRY_RUN:-0}" = 1 ]; then
    if [ "$ref" = latest ]; then
      dry "pnpm add -g $package   # unless already on PATH from \$PNPM_HOME"
    else
      dry "pnpm add -g $package@$(ref_version "$ref")   # unless already at $ref in \$PNPM_HOME"
    fi
    return 0
  fi

  # Three reasons to (re)install, same as node_globals_deduplicate's checks:
  # absent, on PATH but broken (a wrapper whose native binary never arrived),
  # or present but living inside $NVM_DIR -- which works today and vanishes on
  # the next node bump. Only a version mismatch is specific to this step, since
  # NODE_GLOBALS rows have no pin of their own.
  local reinstall=0 where cur
  if ! has "$bin"; then
    reinstall=1
  elif ! runs "$bin"; then
    info "$bin is on PATH but does not run; reinstalling it"
    reinstall=1
  else
    where=$(command -v "$bin")
    case "$where" in
      "$NVM_DIR"/*)
        info "$bin lives in $where; reinstalling it version-independently"
        reinstall=1
        ;;
      *)
        if [ "$ref" != latest ]; then
          cur=$(version_of "$bin")
          [ "$cur" = "$(ref_version "$ref")" ] || reinstall=1
        fi
        ;;
    esac
  fi

  if [ "$reinstall" = 1 ]; then
    local spec="$package"
    [ "$ref" = latest ] || spec="$package@$(ref_version "$ref")"
    info "pnpm add -g $spec"
    run pnpm add -g "$spec"
    path_refresh
  else
    ok "$bin $(version_of "$bin") already matches $ref"
  fi

  # zsh/.zshenv puts nvm's node bin ahead of $PNPM_HOME/bin on PATH, so a
  # leftover `npm -g` copy would keep winning in the shell you actually use --
  # and this step would reinstall on every single run. Same trap and same fix
  # as node_globals_deduplicate in steps/50-runtimes.sh.
  if has npm; then
    local root; root=$(npm root -g 2>/dev/null) || root=""
    if [ -n "$root" ] && [ -d "$root/$package" ]; then
      info "removing the npm -g copy of $package (pnpm owns this now)"
      run npm uninstall -g "$package" >/dev/null 2>&1 || warn "npm uninstall -g reported a problem"
    fi
  fi

  if [ "${DRY_RUN:-0}" != 1 ] && has "$bin"; then
    hash -r 2>/dev/null || true
    info "$bin is now $(version_of "$bin") at $(command -v "$bin")"
  fi
}

_register_npm_global() {
  local name="$1"
  register "npm:$name" no "install npm global $name" "npm_global_install $name"
}
manifest_each npm-globals.tsv _register_npm_global
