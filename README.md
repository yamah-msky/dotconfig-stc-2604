# DotConfig feat. STC

- Author: `mbyamaguchi`

## Usage

### Installation

Install Git, back up any existing configuration you want to keep, then fetch the
repository. The bootstrap installs zsh, configures `ZDOTDIR`, and changes the
login shell; those steps do not need to be performed manually.

```sh
mkdir -p "$HOME/.config"
cd "$HOME/.config"
git init
git remote add origin git@github.com:yamah-msky/dotconfig-stc-2604.git
git fetch origin
git checkout -B main origin/main
```

Finally, run the bootstrap:

```sh
~/.config/bootstrap/bs.sh
exec zsh
```

That installs everything these configs need, at the versions pinned in
`bootstrap/`. It is idempotent, so re-run it any time something looks off.

### Keeping it current

```sh
bootstrap/bs.sh update    # bump every pinned version
git diff                  # review what moved
git commit -am 'Bump pins'
bootstrap/bs.sh           # install it
```

### Checking a machine

```sh
bootstrap/bs.sh doctor    # where does this machine differ from the manifests?
```

`doctor --json` also emits diagnostic codes and remediation text for callers
that want to automate repairs.

### マニュアル

- **[manual/index.html](manual/index.html)** — 詳細マニュアル。ブラウザで開く。
  step 28 個と `doctor` 41 行の全一覧、zsh / neovim / git / tmux の設計、
  トラブルシューティング、落とし穴カタログ
- [bootstrap/MANUAL.md](bootstrap/MANUAL.md) — 端末用の早見表
- [bootstrap/README.md](bootstrap/README.md) — 設計の記録（なぜこの構成なのか）
