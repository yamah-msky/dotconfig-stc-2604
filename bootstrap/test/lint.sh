#!/usr/bin/env bash
# Syntax check and shellcheck every script in the repository.
#
# Uses the local shellcheck when apt:base has installed it, and falls back to the
# official container image otherwise -- so this works before the bootstrap has
# ever run, which is exactly when you want to lint it.

set -euo pipefail

CONFIG_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$CONFIG_DIR"

FILES=(bootstrap/bs.sh bootstrap/lib.sh bootstrap/cleanup.sh
       bootstrap/steps/*.sh bootstrap/test/*.sh yt-dlp/*.sh)

echo "==> bash -n"
for f in "${FILES[@]}"; do
  bash -n "$f" || { echo "FAIL: $f does not parse"; exit 1; }
done
echo "OK: all ${#FILES[@]} files parse"

echo "==> shellcheck"
# SC1091: shellcheck resolves `source` relative to its own cwd, which does not
# match how these files are laid out; -x already follows what it can.
if command -v shellcheck >/dev/null; then
  ( cd bootstrap && shellcheck -x -e SC1091 \
      bs.sh lib.sh cleanup.sh steps/*.sh test/*.sh ../yt-dlp/*.sh )
elif command -v docker >/dev/null; then
  # via sh -c so the globs are expanded inside the container, not passed through
  # as literal arguments
  docker run --rm -v "$CONFIG_DIR:/w:ro" -w /w/bootstrap \
    koalaman/shellcheck-alpine:stable \
    sh -c 'shellcheck -x -e SC1091 bs.sh lib.sh cleanup.sh steps/*.sh test/*.sh ../yt-dlp/*.sh'
else
  echo "SKIP: neither shellcheck nor docker available"
fi
echo "OK: shellcheck clean"

echo "==> go (bootstrap/tool)"
if command -v go >/dev/null; then
  ( cd bootstrap/tool
    gofmt -l . | tee /dev/stderr | grep -q . && { echo "FAIL: gofmt would change the above"; exit 1; }
    go vet ./... || exit 1
    go test ./... || exit 1
  ) || exit 1
  # The tool has to stay buildable by what a bare Ubuntu 26.04 ships, because a
  # fresh machine builds it before anything has upgraded Go.
  if [ -x /usr/lib/go-1.26/bin/go ]; then
    ( cd bootstrap/tool && /usr/lib/go-1.26/bin/go build -o /dev/null . ) \
      || { echo "FAIL: does not build with Ubuntu's Go 1.26"; exit 1; }
    echo "OK: builds with Ubuntu's Go 1.26 as well"
  fi
else
  echo "SKIP: go not installed (run: bootstrap/bs.sh --only go)"
fi
