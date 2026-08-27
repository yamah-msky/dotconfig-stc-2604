return {
  { "nvim-mini/mini.pairs", enabled = false },
  { "garymjr/nvim-snippets", enabled = false },
  {
    "windwp/nvim-autopairs",
    event = "InsertEnter",
    opts = { check_ts = true },
  },
  {
    "kylechui/nvim-surround",
    keys = { "ys", "ds", "cs" },
    version = "*",
    opts = {},
  },
  {
    "nvim-treesitter/nvim-treesitter",
    opts = {
      ensure_installed = {
        "typescript",
        "tsx",
        "javascript",
        "json",
        "yaml",
        "toml",
        "html",
        "css",
        "graphql",
        "lua",
        "vim",
        "vimdoc",
        "markdown",
        "markdown_inline",
        "c",
        "cpp",
        "rust",
        "python",
      },
    },
  },
  {
    "nvim-treesitter/nvim-treesitter-textobjects",
    lazy = false,
    opts = {
      select = { lookahead = true },
      move = { set_jumps = true },
    },
    config = function(_, opts)
      require("nvim-treesitter-textobjects").setup(opts)
      local select = require("nvim-treesitter-textobjects.select")
      local move = require("nvim-treesitter-textobjects.move")

      vim.keymap.set({ "x", "o" }, "af", function()
        select.select_textobject("@function.outer", "textobjects")
      end, { desc = "Outer Function" })
      vim.keymap.set({ "x", "o" }, "if", function()
        select.select_textobject("@function.inner", "textobjects")
      end, { desc = "Inner Function" })
      vim.keymap.set({ "x", "o" }, "ac", function()
        select.select_textobject("@class.outer", "textobjects")
      end, { desc = "Outer Class" })
      vim.keymap.set({ "x", "o" }, "ic", function()
        select.select_textobject("@class.inner", "textobjects")
      end, { desc = "Inner Class" })
      vim.keymap.set("n", "]f", function()
        move.goto_next_start("@function.outer", "textobjects")
      end, { desc = "Next Function" })
      vim.keymap.set("n", "]c", function()
        move.goto_next_start("@class.outer", "textobjects")
      end, { desc = "Next Class" })
      vim.keymap.set("n", "[f", function()
        move.goto_previous_start("@function.outer", "textobjects")
      end, { desc = "Previous Function" })
      vim.keymap.set("n", "[c", function()
        move.goto_previous_start("@class.outer", "textobjects")
      end, { desc = "Previous Class" })
    end,
  },
}
