# DotConfig feat. STC

- Author: `mbyamaguchi`

## Usage

### Installation

Firstly, install zsh (run `sudo apt install zsh` or something else) and change your default shell to the zsh (consider to run `chsh -s /bin/zsh`).

Next add below to the bottom of `/etc/zsh/zshenv`
```zshenv
ZDOTDIR=$HOME/.config/zsh
```

Then run `cd $HOME/.config` on your shell to move to the working dir.

After setting up ssh for your github account, run some commands below.

Do not forget to save backups like what you want.

```sh
git init
git remote add origin git@github.com:mbyamaguchi/dotconfig-stc.git
git checkout main
git fetch
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

### マニュアル

- **[manual/index.html](manual/index.html)** — 詳細マニュアル。ブラウザで開く。
  step 28 個と `doctor` 41 行の全一覧、zsh / neovim / git / tmux の設計、
  トラブルシューティング、落とし穴カタログ
- [bootstrap/MANUAL.md](bootstrap/MANUAL.md) — 端末用の早見表
- [bootstrap/README.md](bootstrap/README.md) — 設計の記録（なぜこの構成なのか）
