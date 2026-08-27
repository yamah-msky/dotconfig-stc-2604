local map = function(mode, lhs, rhs, opts)
  opts = opts or {}
  opts.silent = opts.silent ~= false
  vim.keymap.set(mode, lhs, rhs, opts)
end

-- Most navigation, window, buffer and diagnostic mappings come from LazyVim.
map("i", "jk", "<Esc>", { desc = "Normal Mode" })
map("v", "p", '"_dP', { desc = "Paste Without Yanking" })

-- Keep the existing buffer key, but use LazyVim's layout-safe deletion.
map("n", "<leader>bd", function()
  Snacks.bufdelete()
end, { desc = "Delete Buffer" })
