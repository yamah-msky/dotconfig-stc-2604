#!/usr/bin/env bash
# =============================================================================
# bootstrap/test/container.sh
# Run the whole bootstrap in a clean ubuntu:26.04 container and check the result.
#
#   ./container.sh          full run, including the neovim plugin restore
#   ./container.sh --fast   skip nvim:plugins (minutes of compiling) while iterating
#   ./container.sh --head   test the committed tree instead of tracked worktree files
#   ./container.sh --shell  drop into a shell in the container instead of testing
#
# This is the only honest test of "would a fresh machine end up like this one".
# The container gets a non-root user with passwordless sudo, so the four steps
# that need root are genuinely exercised rather than skipped -- and it has no
# /mnt/* and no WSL, so the WSL handling self-skips, which tests that path too.
#
# Only tracked files go in. By default their current worktree contents are used,
# so a change can be tested before commit; --head reproduces an exact fresh clone.
# Untracked tokens, browser profiles and shell history never enter the container.
# =============================================================================

set -euo pipefail

CONFIG_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE=dotconfig-bootstrap-test
MODE=full
SKIP_ARG=""
USE_HEAD=0

while [ $# -gt 0 ]; do
  case "$1" in
    --fast)  SKIP_ARG="--skip nvim:plugins"; shift ;;
    --head)  USE_HEAD=1; shift ;;
    --shell) MODE=shell; shift ;;
    -h|--help) sed -n '3,20p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }

archive_input() {
  if [ "$USE_HEAD" = 1 ]; then
    git -C "$CONFIG_DIR" archive HEAD
    return
  fi
  git -C "$CONFIG_DIR" ls-files --cached -z \
    | tar -C "$CONFIG_DIR" --null --ignore-failed-read -T - -cf -
}

echo "==> building $IMAGE"
docker build -q -t "$IMAGE" -f "$CONFIG_DIR/bootstrap/test/Dockerfile" \
  "$CONFIG_DIR/bootstrap/test"

# Everything below reads the selected repository snapshot from stdin, unpacks it
# into ~/.config, and makes it a git repo again so the "nothing was modified"
# check has something to compare against.
# Single-quoted: this runs inside the container, where $HOME is the container's.
# shellcheck disable=SC2016
SEED='
set -uo pipefail
mkdir -p "$HOME/.config"
tar -x -C "$HOME/.config"
git -C "$HOME/.config" init -q -b main
git -C "$HOME/.config" -c user.email=t@example.com -c user.name=t \
    -c commit.gpgsign=false add -A
git -C "$HOME/.config" -c user.email=t@example.com -c user.name=t \
    -c commit.gpgsign=false commit -q -m seed
'

if [ "$MODE" = shell ]; then
  archive_input \
    | exec docker run --rm -i -e BOOTSTRAP_TEST_NO_SYSTEMD=1 "$IMAGE" \
        bash -lc "$SEED"'; cd ~/.config; exec bash'
fi

echo "==> running bootstrap in a clean container"
archive_input \
  | docker run --rm -i -e "SKIP_ARG=$SKIP_ARG" -e BOOTSTRAP_TEST_NO_SYSTEMD=1 \
      "$IMAGE" bash -lc "$SEED"'
echo "--- bs.sh --dry-run (must change nothing) -------------------------------"
"$HOME/.config/bootstrap/bs.sh" --dry-run >/dev/null || { echo "FAIL: dry run"; exit 1; }
[ ! -e "$HOME/.local" ] || { echo "FAIL: dry run created ~/.local"; exit 1; }
if find /tmp -maxdepth 1 -type d -name "dotconfig-bootstrap.*" | grep -q .; then
  echo "FAIL: dry run left a temporary directory"; exit 1
fi

echo "--- bs.sh --dry-run --arch aarch64 (must skip, not fail) ----------------"
"$HOME/.config/bootstrap/bs.sh" --dry-run --arch riscv64 >/dev/null \
  || { echo "FAIL: unknown arch should skip, not fail"; exit 1; }

echo "--- doctor before Go exists must say so, not crash ---------------------"
# Nothing has installed Go yet, and doctor is written in Go. The failure has to
# name the fix rather than dying in a build error.
if out=$("$HOME/.config/bootstrap/bs.sh" doctor 2>&1); then
  echo "FAIL: doctor should not succeed before Go is installed"; exit 1
fi
printf "%s" "$out" | grep -q -- "--only go" \
  || { echo "FAIL: doctor did not point at the fix. Got:"; printf "%s\n" "$out"; exit 1; }
echo "OK: doctor explains it needs Go first"

echo "--- bs.sh (full install) -----------------------------------------------"
# shellcheck disable=SC2086
"$HOME/.config/bootstrap/bs.sh" $SKIP_ARG || { echo "FAIL: install reported failures"; exit 1; }

echo "--- bs.sh (second run: must download nothing) --------------------------"
# shellcheck disable=SC2086
out=$("$HOME/.config/bootstrap/bs.sh" $SKIP_ARG 2>&1)
# Match the "-> <url>" line lib.sh logs before each download. A bare grep for
# https:// also catches sheldon reporting the revisions it verified, which is not
# a download.
if printf "%s" "$out" | grep -qE "^ +-> https://"; then
  echo "FAIL: not idempotent -- the second run downloaded something:"
  printf "%s\n" "$out" | grep -E "^ +-> https://"
  exit 1
fi
echo "OK: no downloads on the second run"

echo "--- the repository must be unmodified ----------------------------------"
if ! git -C "$HOME/.config" diff --quiet; then
  echo "FAIL: bootstrap modified tracked files:"
  git -C "$HOME/.config" diff
  exit 1
fi
echo "OK: no tracked file was touched"
if find /tmp -maxdepth 1 -type d -name "dotconfig-bootstrap.*" | grep -q .; then
  echo "FAIL: bootstrap left a temporary directory"; exit 1
fi
echo "OK: temporary directories were cleaned"

echo "--- zsh must start cleanly ---------------------------------------------"
# "can.t change option: zle" comes from fzf.s zsh integration when there is no
# terminal attached, which is true of `zsh -ic` under docker. It happens on a
# working machine too, so it is filtered rather than reported.
noise="can.t change option: zle"
zsh_out=$(zsh -ic exit 2>&1 | grep -vE "$noise") || true
if [ -n "$zsh_out" ]; then
  echo "FAIL: zsh printed unexpected output on startup:"
  printf "%s\n" "$zsh_out"
  exit 1
fi
echo "OK: no unexpected startup output"

echo "--- the contracts zsh/.zshrc depends on --------------------------------"
for c in "fzf --zsh" "starship --version" "zoxide --version" "eza --version" \
         "stylua --version" "nvim --version"; do
  zsh -ic "$c >/dev/null" 2>/dev/null || { echo "FAIL: $c"; exit 1; }
done
echo "OK: prompt, fuzzy finder, jump, ls, formatter and editor all resolve"

echo "--- bs.sh doctor -------------------------------------------------------"
if [ -n "$SKIP_ARG" ]; then
  BOOTSTRAP_TEST_SKIP_NVIM_PLUGINS=1 "$HOME/.config/bootstrap/bs.sh" doctor
else
  "$HOME/.config/bootstrap/bs.sh" doctor
fi
exit $?
'
echo "==> container test finished"
