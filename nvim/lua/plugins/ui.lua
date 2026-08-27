return {
  {
    "LazyVim/LazyVim",
    opts = { colorscheme = "monokai-pro" },
  },
  {
    "loctvl842/monokai-pro.nvim",
    priority = 1000,
    opts = {
      devicons = true,
      filter = "pro",
    },
  },
  {
    "nvim-lualine/lualine.nvim",
    opts = function(_, opts)
      opts.options = vim.tbl_deep_extend("force", opts.options or {}, {
        theme = "monokai-pro",
        globalstatus = true,
      })
      opts.sections = opts.sections or {}
      opts.sections.lualine_c = { { "filename", path = 1 } }

      local lsp = {
        function()
          local names = vim.tbl_map(function(client)
            return client.name
          end, vim.lsp.get_clients({ bufnr = 0 }))
          return #names > 0 and (" " .. table.concat(names, ", ")) or ""
        end,
        color = { fg = "#7aa2f7" },
      }
      opts.sections.lualine_x = { lsp, "encoding", "fileformat", "filetype" }
    end,
  },
  {
    "folke/snacks.nvim",
    keys = {
      { "<leader>e", false },
      { "<leader>E", false },
    },
    opts = { explorer = { enabled = false } },
  },
  {
    "nvim-tree/nvim-tree.lua",
    dependencies = { "nvim-tree/nvim-web-devicons" },
    cmd = { "NvimTreeToggle", "NvimTreeOpen", "NvimTreeFindFile" },
    keys = { { "<leader>e", "<cmd>NvimTreeToggle<cr>", desc = "File Tree" } },
    opts = {
      filters = { dotfiles = false },
      renderer = { group_empty = true },
    },
  },
  {
    "lukas-reineke/indent-blankline.nvim",
    opts = {
      indent = { char = "|" },
      scope = { enabled = true },
    },
  },
}
