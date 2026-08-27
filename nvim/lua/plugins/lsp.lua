local ts_inlay_hints = {
  includeInlayParameterNameHints = "all",
  includeInlayParameterNameHintsWhenArgumentMatchesName = false,
  includeInlayFunctionParameterTypeHints = true,
  includeInlayVariableTypeHints = true,
  includeInlayVariableTypeHintsWhenTypeMatchesName = false,
  includeInlayPropertyDeclarationTypeHints = true,
  includeInlayFunctionLikeReturnTypeHints = true,
  includeInlayEnumMemberValueHints = true,
}

return {
  {
    "neovim/nvim-lspconfig",
    opts = function(_, opts)
      local severity = vim.diagnostic.severity
      opts.diagnostics = vim.tbl_deep_extend("force", opts.diagnostics or {}, {
        virtual_text = { prefix = "●", source = "if_many" },
        float = { border = "rounded", source = true },
        signs = {
          text = {
            [severity.ERROR] = " ",
            [severity.WARN] = " ",
            [severity.HINT] = " ",
            [severity.INFO] = " ",
          },
          numhl = {
            [severity.ERROR] = "DiagnosticSignError",
            [severity.WARN] = "DiagnosticSignWarn",
            [severity.HINT] = "DiagnosticSignHint",
            [severity.INFO] = "DiagnosticSignInfo",
          },
        },
        jump = {
          on_jump = function(_, bufnr)
            vim.diagnostic.open_float({ bufnr = bufnr, scope = "cursor", focus = false })
          end,
        },
      })
      -- lua-language-server can return a nil inlayHint result on Neovim 0.12,
      -- which currently raises an error in the built-in response handler.
      opts.inlay_hints = { enabled = true, exclude = { "lua" } }
      opts.servers = opts.servers or {}
      opts.servers.vtsls = vim.tbl_deep_extend("force", opts.servers.vtsls or {}, {
        settings = {
          typescript = { inlayHints = ts_inlay_hints },
          javascript = { inlayHints = ts_inlay_hints },
        },
      })
      opts.servers.cssls = opts.servers.cssls or {}
      opts.servers.html = opts.servers.html or {}
      opts.servers.emmet_ls = {
        filetypes = { "html", "css", "scss", "javascriptreact", "typescriptreact" },
      }
      opts.servers.pyright = vim.tbl_deep_extend("force", opts.servers.pyright or {}, {
        settings = {
          python = {
            analysis = {
              autoSearchPaths = true,
              diagnosticMode = "workspace",
              useLibraryCodeForTypes = true,
              typeCheckingMode = "basic",
            },
          },
        },
      })

      opts.servers["*"] = opts.servers["*"] or {}
      opts.servers["*"].keys = vim.list_extend(opts.servers["*"].keys or {}, {
        { "gD", vim.lsp.buf.declaration, desc = "Goto Declaration", has = "declaration" },
        { "gi", vim.lsp.buf.implementation, desc = "Goto Implementation", has = "implementation" },
        { "gt", vim.lsp.buf.type_definition, desc = "Goto Type Definition", has = "typeDefinition" },
        { "<C-k>", vim.lsp.buf.signature_help, desc = "Signature Help", has = "signatureHelp" },
        { "<leader>rn", vim.lsp.buf.rename, desc = "Rename", has = "rename" },
        { "<leader>d", vim.diagnostic.open_float, desc = "Diagnostic Details" },
        {
          "<leader>ih",
          function()
            local bufnr = vim.api.nvim_get_current_buf()
            vim.lsp.inlay_hint.enable(not vim.lsp.inlay_hint.is_enabled({ bufnr = bufnr }), { bufnr = bufnr })
          end,
          desc = "Toggle Inlay Hints",
        },
      })
    end,
  },
  {
    "mason-org/mason.nvim",
    opts = function(_, opts)
      opts.ensure_installed = opts.ensure_installed or {}
      opts.ensure_installed = LazyVim.dedup(vim.list_extend(opts.ensure_installed, { "clang-format" }))
    end,
  },
}
