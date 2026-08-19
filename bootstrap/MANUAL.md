# bootstrap 早見表

**詳細マニュアルは [`manual/index.html`](../manual/index.html) にあります**（ブラウザで開く）。
step 28 個と `doctor` 41 行の全一覧、zsh / neovim / git の設計、落とし穴カタログはそちら。
このファイルは端末で `cat` する用の早見表で、設計の記録は [README.md](README.md)（英語）。

## 覚えるコマンドは 3 つ

```sh
~/.config/bootstrap/bs.sh          # 環境を再現・修復する（冪等）
~/.config/bootstrap/bs.sh update   # 全バージョンを最新にする → git diff を見て commit
~/.config/bootstrap/bs.sh doctor   # 今のマシンが manifest とどこで違うかを表で出す
```

`bs.sh list` で step 一覧と root 要否が出る。**`bs.sh` は commit された manifest に
書いてあるものしか入れない。** バージョンが動くのは `update` を実行したときだけ。

## 新規マシン

```sh
sudo apt update && sudo apt install -y git zsh
cd "$HOME/.config" && git init
git remote add origin git@github.com:mbyamaguchi/dotconfig-stc.git
git fetch && git checkout main
./bootstrap/bs.sh        # sudo は最初に 1 度だけ聞かれる。10〜20 分
# 初回だけログアウト/再ログイン（または newgrp docker）して Docker 権限を反映
exec zsh
```

sudo が無い環境では `--no-sudo`（root 必須の 8 step を飛ばす）。
**`sudo ./bootstrap/bs.sh` は不可** — `/root/.local` に入るのでスクリプトが拒否する。

## 部分的に流す

```sh
bs.sh --only tool:              # tools.tsv の全ツール（ID は前方一致）
bs.sh --only tool:fzf           # 1 つだけ
bs.sh --skip nvim:plugins       # 遅いものを飛ばす
bs.sh --dry-run                 # 何が起きるか表示するだけ
bs.sh update --check            # 上流に遅れているものだけ表示
```

## 困ったとき

| 症状 | 対処 |
|---|---|
| `doctor` が WARN を出す | 多くは `bs.sh` の再実行で直る。行ごとの対処は詳細マニュアル §5 |
| `doctor` / `update` が「Go が必要」と言う | `bs.sh --only go`（この 2 つは Go 製） |
| ハイライトが無い / `nvim:parsers` が WARN | `bs.sh --only pnpm` の後に `bs.sh --only nvim:plugins` |
| `locale` / `zdotdir` が FAIL | `bs.sh --only locale` / `--only zdotdir`（sudo 必要） |
| `docker:access` が WARN | `bs.sh --only docker` 後なら `newgrp docker` または再ログイン。`docker` グループは root 相当権限を持つ |
| プラグインが変わった | `bs.sh --only nvim:plugins` が lockfile の revision へ戻す |
| `npm install -g` した CLI が消えた | node バージョン内に入っていて bump で PATH から外れただけ。`npm-globals.tsv` に一行足して `bs.sh --only npm:<name>` で `$PNPM_HOME` 配下に入れ直す |
| 実行が途中で失敗した | 1 step の失敗で全体は止まらない。最後のサマリに再実行コマンドが出る |

## テスト

```sh
bootstrap/test/lint.sh         # bash -n / shellcheck / gofmt / go vet / go test
bootstrap/test/container.sh    # クリーンな ubuntu:26.04 でフル実走（--fast で高速化）
```
