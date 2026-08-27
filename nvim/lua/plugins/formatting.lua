return {
  {
    "stevearc/conform.nvim",
    opts = function(_, opts)
      opts.formatters_by_ft = vim.tbl_deep_extend("force", opts.formatters_by_ft or {}, {
        c = { "clang_format" },
        cpp = { "clang_format" },
        rust = { "rustfmt" },
        python = { "ruff_format" },
      })
      opts.formatters = opts.formatters or {}
      opts.formatters.prettier = vim.tbl_deep_extend("force", opts.formatters.prettier or {}, {
        prepend_args = function(_, ctx)
          local config_files = {
            ".prettierrc",
            ".prettierrc.js",
            ".prettierrc.cjs",
            ".prettierrc.json",
            ".prettierrc.yaml",
            ".prettierrc.yml",
            "prettier.config.js",
            "prettier.config.cjs",
            "prettier.config.mjs",
          }
          if #vim.fs.find(config_files, { path = ctx.dirname, upward = true }) > 0 then
            return {}
          end
          return {
            "--tab-width",
            "2",
            "--single-quote",
            "true",
            "--trailing-comma",
            "es5",
            "--print-width",
            "100",
            "--semi",
            "true",
          }
        end,
      })
    end,
  },
}
