# A machine as close to "fresh Ubuntu" as the bootstrap can be asked to handle:
# only what you would have before running it, plus sudo so the four root steps
# are really tested instead of skipped.
FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -y \
 && apt-get install -y --no-install-recommends sudo git ca-certificates curl zsh \
 && rm -rf /var/lib/apt/lists/*

# A normal user, because bootstrap installs into $HOME and refuses to run as root
# via sudo. NOPASSWD so the run is unattended.
RUN useradd -m -s /bin/bash tester \
 && echo 'tester ALL=(ALL) NOPASSWD:ALL' >/etc/sudoers.d/tester \
 && chmod 0440 /etc/sudoers.d/tester

USER tester
WORKDIR /home/tester
ENV HOME=/home/tester
