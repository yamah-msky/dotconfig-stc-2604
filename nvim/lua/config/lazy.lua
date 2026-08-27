local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"
if not (vim.uv or vim.loop).fs_stat(lazypath) then
  local lazyrepo = "https://github.com/folke/lazy.nvim.git"

  -- lazy-lock.json pins lazy.nvim along with everything else, so clone that
  -- exact commit instead of whatever the `stable` tag points at today.
  -- 一致させないと、新規マシンでは lazy.nvim 自身だけがロックファイルと
  -- 違う版になり、起動時に lazy が lazy-lock.json を書き換えてしまう。
  local pinned
  do
    local f = io.open(vim.fn.stdpath("config") .. "/lazy-lock.json", "r")
    if f then
      local ok, lock = pcall(vim.json.decode, f:read("*a"))
      f:close()
      if ok and type(lock) == "table" and lock["lazy.nvim"] then
        pinned = lock["lazy.nvim"].commit
      end
    end
  end

  local args = { "git", "clone", "--filter=blob:none", lazyrepo, lazypath }
  if not pinned then
    -- ロックファイルが無いとき（まったく新規の環境）だけ stable にフォールバック
    table.insert(args, 5, "--branch=stable")
  end

  local out = vim.fn.system(args)
  if vim.v.shell_error == 0 and pinned then
    out = vim.fn.system({ "git", "-C", lazypath, "checkout", "--quiet", pinned })
  end
  if vim.v.shell_error ~= 0 then
    vim.api.nvim_echo({
      { "Failed to clone lazy.nvim:\n", "ErrorMsg" },
      { out, "WarningMsg" },
      { "\nPress any key to exit..." },
    }, true, {})
    vim.fn.getchar()
    os.exit(1)
  end
end
vim.opt.rtp:prepend(lazypath)

require("lazy").setup({
  spec = {
    { "LazyVim/LazyVim", import = "lazyvim.plugins" },
    { import = "lazyvim.plugins.extras.editor.telescope" },
    { import = "lazyvim.plugins.extras.coding.nvim-cmp" },
    { import = "lazyvim.plugins.extras.ui.indent-blankline" },
    { import = "lazyvim.plugins.extras.lang.typescript" },
    { import = "lazyvim.plugins.extras.lang.json" },
    { import = "lazyvim.plugins.extras.lang.clangd" },
    { import = "lazyvim.plugins.extras.lang.rust" },
    { import = "lazyvim.plugins.extras.lang.python" },
    { import = "lazyvim.plugins.extras.linting.eslint" },
    { import = "lazyvim.plugins.extras.formatting.prettier" },
    { import = "plugins" },
  },
  defaults = {
    lazy = false,
    version = false,
  },
  install = { colorscheme = { "monokai-pro", "habamax" } },
  checker = { enabled = true, notify = false },
  rocks = { enabled = false },
  performance = {
    rtp = {
      disabled_plugins = {
        "gzip",
        "netrwPlugin",
        "tarPlugin",
        "tohtml",
        "tutor",
        "zipPlugin",
      },
    },
  },
})
