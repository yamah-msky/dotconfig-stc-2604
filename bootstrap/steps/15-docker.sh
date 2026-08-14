# shellcheck shell=bash
# Docker Engine from Docker's official apt repository.
#
# This is deliberately a bespoke step rather than an apt.tsv group: the
# packages do not exist until their signed first-party repository is configured.
# Existing images, containers, volumes and daemon configuration are never
# touched. Conflicting distro packages are reported, not removed automatically.

DOCKER_KEYRING=/etc/apt/keyrings/docker.asc
DOCKER_SOURCE=/etc/apt/sources.list.d/docker.sources
DOCKER_PACKAGES=(docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin)
DOCKER_CONFLICTS=(docker.io docker-compose docker-compose-v2 docker-doc docker-buildx podman-docker containerd runc)

docker_repo_present() {
  grep -Rqs 'download\.docker\.com/linux/ubuntu' \
    /etc/apt/sources.list /etc/apt/sources.list.d 2>/dev/null
}

docker_repo_key_path() {
  local f key
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/*; do
    [ -r "$f" ] || continue
    grep -qs 'download\.docker\.com/linux/ubuntu' "$f" || continue
    key=$(sed -nE 's/.*[Ss]igned-[Bb]y[=:][[:space:]]*([^] ]+).*/\1/p' "$f" | head -n1)
    [ -n "$key" ] && { printf '%s\n' "$key"; return 0; }
  done
  return 1
}

docker_conflicts_installed() {
  local p
  for p in "${DOCKER_CONFLICTS[@]}"; do
    dpkg-query -W -f='${Status}' "$p" 2>/dev/null | grep -q 'ok installed' && printf '%s\n' "$p"
  done
  return 0
}

docker_configure_repo() {
  if docker_repo_present; then
    local existing_key
    existing_key=$(docker_repo_key_path) || {
      warn "existing Docker apt source has no Signed-By key"
      return 1
    }
    if [ ! -s "$existing_key" ]; then
      warn "Docker apt source references a missing key: $existing_key"
      warn "repair or remove that source before re-running the bootstrap"
      return 1
    fi
    ok "Docker apt repository already configured ($existing_key)"
    return 0
  fi

  local os_id codename arch tmp
  os_id=$(. /etc/os-release && printf '%s' "${ID:-}")
  codename=$(. /etc/os-release && printf '%s' "${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}")
  arch=$(dpkg --print-architecture)
  [ "$os_id" = ubuntu ] || { warn "Docker CE step supports Ubuntu only (found ${os_id:-unknown})"; return 1; }
  [ -n "$codename" ] || { warn "could not determine the Ubuntu codename"; return 1; }

  info "adding Docker's official apt repository ($codename/$arch)"
  run_sudo install -m 0755 -d /etc/apt/keyrings
  run_sudo curl "${CURL_OPTS[@]}" https://download.docker.com/linux/ubuntu/gpg -o "$DOCKER_KEYRING"
  run_sudo chmod a+r "$DOCKER_KEYRING"

  if [ "${DRY_RUN:-0}" = 1 ]; then
    dry "install Docker deb822 source -> $DOCKER_SOURCE"
  else
    tmp=$(mktmp)
    printf '%s\n' \
      'Types: deb' \
      'URIs: https://download.docker.com/linux/ubuntu' \
      "Suites: $codename" \
      'Components: stable' \
      "Architectures: $arch" \
      "Signed-By: $DOCKER_KEYRING" >"$tmp/docker.sources"
    run_sudo install -m 0644 "$tmp/docker.sources" "$DOCKER_SOURCE"
  fi
  _APT_UPDATED=0
}

docker_install_packages() {
  local p missing=""
  for p in "${DOCKER_PACKAGES[@]}"; do
    dpkg-query -W -f='${Status}' "$p" 2>/dev/null | grep -q 'ok installed' || missing+=" $p"
  done
  if [ -z "$missing" ]; then
    ok "Docker Engine, CLI, Buildx and Compose packages installed"
    return 0
  fi
  info "installing$missing"
  apt_refresh || return 1
  # shellcheck disable=SC2086
  DEBIAN_FRONTEND=noninteractive run_sudo apt-get install -y $missing
}

docker_configure_access() {
  local user
  user=$(id -un)
  [ "$user" = root ] && return 0
  if id -nG "$user" 2>/dev/null | tr ' ' '\n' | grep -qx docker; then
    ok "$user is registered in the docker group"
  else
    info "adding $user to the docker group"
    run_sudo usermod -aG docker "$user"
  fi
}

step_docker() {
  local conflicts
  conflicts=$(docker_conflicts_installed)
  if [ -n "$conflicts" ]; then
    conflicts=${conflicts//$'\n'/ }
    warn "Docker CE conflicts with installed package(s): $conflicts"
    warn "review and remove them explicitly, then re-run: sudo apt-get remove $conflicts"
    return 1
  fi

  docker_configure_repo
  docker_install_packages
  docker_configure_access

  # The clean-container test has no systemd as PID 1. It still exercises the
  # repository, package and account setup; live hosts must exercise the daemon.
  if [ "${BOOTSTRAP_TEST_NO_SYSTEMD:-0}" = 1 ]; then
    skip "container test: service and daemon checks need systemd"
  else
    has systemctl || { warn "systemctl is required to manage Docker Engine"; return 1; }
    info "enabling and starting Docker Engine"
    run_sudo systemctl enable --now containerd.service docker.service
    if [ "${DRY_RUN:-0}" != 1 ]; then
      run_sudo docker info >/dev/null
      docker buildx version >/dev/null
      docker compose version >/dev/null
      ok "Docker daemon, Buildx and Compose respond"
    fi
  fi

  if [ "${DRY_RUN:-0}" != 1 ] && ! id -nG | tr ' ' '\n' | grep -qx docker; then
    warn "docker group membership is registered but not active in this shell"
    warn "run 'newgrp docker', or log out and back in (restart WSL if needed)"
  fi
}

register docker yes "Docker Engine, Buildx and Compose from Docker's apt repository" step_docker
