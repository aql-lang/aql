-- boru Language Server for Neovim.
--
-- This file covers both the modern (Neovim 0.11+) vim.lsp.config API
-- and the historical nvim-lspconfig package. Pick whichever matches
-- your setup; do not load both.

-- ---------------------------------------------------------------------
-- Filetype detection: associate *.boru with the "boru" filetype so the
-- LSP attaches via filetype = { "boru" }.
-- ---------------------------------------------------------------------

vim.filetype.add({ extension = { boru = "boru" } })

-- ---------------------------------------------------------------------
-- Option A: Neovim 0.11+ — vim.lsp.config (the new built-in API).
-- Drop this into ~/.config/nvim/init.lua (or a file sourced from it).
-- ---------------------------------------------------------------------

vim.lsp.config("boru", {
  cmd = { "boru", "lsp" },
  filetypes = { "boru" },
  root_markers = { "boru.jsonic", ".git" },
})

vim.lsp.enable("boru")

-- ---------------------------------------------------------------------
-- Option B: nvim-lspconfig (legacy / supported alternative).
-- Requires neovim/nvim-lspconfig installed via your plugin manager.
-- Uncomment if you use lspconfig and remove the block above.
-- ---------------------------------------------------------------------

-- local lspconfig = require("lspconfig")
-- local configs   = require("lspconfig.configs")
--
-- if not configs.boru then
--   configs.boru = {
--     default_config = {
--       cmd       = { "boru", "lsp" },
--       filetypes = { "boru" },
--       root_dir  = lspconfig.util.root_pattern("boru.jsonic", ".git"),
--       settings  = {},
--     },
--   }
-- end
--
-- lspconfig.boru.setup({})
