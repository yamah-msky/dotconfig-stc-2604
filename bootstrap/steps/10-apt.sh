# shellcheck shell=bash
# apt packages, the GitHub CLI repository, and the two Ubuntu-only name shims.

# A file under the run-scoped temporary root survives the per-step subshells.
# A shell variable did not: on a fresh machine every apt group ran apt-get
# update again even though _APT_UPDATED appeared to cache it.
apt_state_file() { printf '%s/apt-updated\n' "$RUN_TMP_ROOT"; }

apt_invalidate() {
  [ -n "${RUN_TMP_ROOT:-}" ] && rm -f -- "$(apt_state_file)"
  return 0
}

apt_refresh() {
  [ "${DRY_RUN:-0}" != 1 ] && [ -f "$(apt_state_file)" ] && return 0
  run_sudo apt-get update -y || return 1
  [ "${DRY_RUN:-0}" = 1 ] || : >"$(apt_state_file)"
}

# Everything in one apt-get invocation per group: apt resolves them together and
# it is far faster than one call per package.
apt_group() {
  local group="$1" pkgs
  pkgs=$(awk -F'\t' -v g="$group" '!/^#/ && NF && $1 == g { print $2 }' "$BOOTSTRAP_DIR/apt.tsv" \
         | tr '\n' ' ')
  [ -n "${pkgs// /}" ] || { warn "apt.tsv has no group '$group'"; return 1; }

  # dpkg-query is much cheaper than letting apt-get re-resolve an already
  # satisfied set, and it keeps re-runs quiet.
  local p missing=""
  for p in $pkgs; do
    dpkg-query -W -f='${Status}' "$p" 2>/dev/null | grep -q 'ok installed' || missing+=" $p"
  done
  if [ -z "$missing" ]; then
    ok "$group: all $(printf '%s' "$pkgs" | wc -w) packages present"
    return 0
  fi

  info "$group: installing$missing"
  apt_refresh || return 1
  # shellcheck disable=SC2086
  DEBIAN_FRONTEND=noninteractive run_sudo apt-get install -y $missing
}

step_apt_base()  { apt_group base; apt_shims; }
step_apt_cpp()   { apt_group cpp; }
step_apt_media() { apt_group media; }

# Ubuntu ships these under different names than every config expects, because
# `fd` and `bat` collide with other Debian packages. The configs call `fd` and
# `bat` (see zsh/aliases.zsh and the zsh-bat plugin), so bridge the names.
apt_shims() {
  local src
  if ! has fd && has fdfind; then
    src=$(command -v fdfind); run ln -sf "$src" "$LOCAL_BIN/fd"; info "fd -> $src"
  fi
  if ! has bat && has batcat; then
    src=$(command -v batcat); run ln -sf "$src" "$LOCAL_BIN/bat"; info "bat -> $src"
  fi
}

# gh is not in Ubuntu's archive at a useful version, and its own apt repository
# is the officially supported route, so it gets one exception to "no third-party
# repos" -- it is first-party for the tool in question and keyring-signed.
GH_KEYRING=/etc/apt/keyrings/githubcli-archive-keyring.gpg
GH_LIST=/etc/apt/sources.list.d/github-cli.list

step_gh() {
  if has gh; then
    ok "gh $(version_of gh) already installed"
    return 0
  fi
  if [ ! -s "$GH_KEYRING" ]; then
    info "adding the GitHub CLI apt repository"
    run_sudo mkdir -p -m 755 /etc/apt/keyrings
    if [ "${DRY_RUN:-0}" = 1 ]; then
      dry "curl https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo tee $GH_KEYRING"
      dry "echo 'deb ... https://cli.github.com/packages stable main' | sudo tee $GH_LIST"
    else
      curl "${CURL_OPTS[@]}" https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | ${SUDO_CMD:+$SUDO_CMD} tee "$GH_KEYRING" >/dev/null
      run_sudo chmod go+r "$GH_KEYRING"
      printf 'deb [arch=%s signed-by=%s] https://cli.github.com/packages stable main\n' \
        "$(dpkg --print-architecture)" "$GH_KEYRING" \
        | ${SUDO_CMD:+$SUDO_CMD} tee "$GH_LIST" >/dev/null
      apt_invalidate
    fi
  fi
  apt_refresh || return 1
  DEBIAN_FRONTEND=noninteractive run_sudo apt-get install -y gh
}

register apt:base  yes "base apt packages, plus the fd/bat name shims" step_apt_base
register apt:cpp   yes "C/C++ toolchain (clangd, clang-format, cmake, ...)" step_apt_cpp
register apt:media yes "ffmpeg (yt-dlp needs it to merge streams)" step_apt_media
register gh        yes "GitHub CLI, from its own apt repository" step_gh
