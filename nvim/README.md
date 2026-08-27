# Neovim / LazyVim

Personal Neovim configuration based on [LazyVim](https://www.lazyvim.org/).

## Requirements

- Neovim 0.12 or newer
- Git, ripgrep, fd and make
- A Nerd Font

Language servers and most command-line tools are installed through Mason. Rust formatting uses the `rustfmt` component from rustup.

## Installation

```sh
git clone <repository-url> ~/.config/nvim
nvim
```

The first launch installs LazyVim, plugins, Treesitter parsers and language tooling. Plugin revisions are pinned in `lazy-lock.json`.

## Included language support

- TypeScript / JavaScript: vtsls, ESLint and Prettier
- HTML / CSS: html-lsp, css-lsp and Emmet
- Lua: lua-language-server and StyLua
- C / C++: clangd and clang-format
- Rust: rustaceanvim, rust-analyzer and rustfmt
- Python: Pyright, Ruff LSP and Ruff formatter

Telescope, nvim-cmp and nvim-tree are retained in place of LazyVim's default picker, completion engine and explorer.

## Maintenance

- `:Lazy` updates and inspects plugins.
- `:LazyExtras` shows available LazyVim Extras.
- `:LazyHealth` and `:checkhealth` diagnose the installation.
- `stylua --check init.lua lua` checks Lua formatting.
