-- These files are loaded automatically by LazyVim before plugin setup.
vim.g.lazyvim_picker = "telescope"
vim.g.lazyvim_cmp = "nvim-cmp"
vim.g.lazyvim_ts_lsp = "vtsls"
vim.g.lazyvim_python_lsp = "pyright"
vim.g.lazyvim_python_ruff = "ruff"
vim.g.lazyvim_rust_diagnostics = "rust-analyzer"

local opt = vim.opt

-- Keep only preferences that differ from, or intentionally document, LazyVim defaults.
opt.relativenumber = true
opt.cursorline = true
opt.wrap = false
opt.scrolloff = 8
opt.sidescrolloff = 8
opt.tabstop = 2
opt.softtabstop = 2
opt.shiftwidth = 2
opt.expandtab = true
opt.pumheight = 10
