-- AQL Language Server for Neovim.
--
-- This file covers both the modern (Neovim 0.11+) vim.lsp.config API
-- and the historical nvim-lspconfig package. Pick whichever matches
-- your setup; do not load both.

-- ---------------------------------------------------------------------
-- Filetype detection: associate *.aql with the "aql" filetype so the
-- LSP attaches via filetype = { "aql" }.
-- ---------------------------------------------------------------------

vim.filetype.add({ extension = { aql = "aql" } })

-- ---------------------------------------------------------------------
-- Option A: Neovim 0.11+ — vim.lsp.config (the new built-in API).
-- Drop this into ~/.config/nvim/init.lua (or a file sourced from it).
-- ---------------------------------------------------------------------

vim.lsp.config("aql", {
  cmd = { "aql", "lsp" },
  filetypes = { "aql" },
  root_markers = { "aql.jsonic", ".git" },
})

vim.lsp.enable("aql")

-- ---------------------------------------------------------------------
-- Option B: nvim-lspconfig (legacy / supported alternative).
-- Requires neovim/nvim-lspconfig installed via your plugin manager.
-- Uncomment if you use lspconfig and remove the block above.
-- ---------------------------------------------------------------------

-- local lspconfig = require("lspconfig")
-- local configs   = require("lspconfig.configs")
--
-- if not configs.aql then
--   configs.aql = {
--     default_config = {
--       cmd       = { "aql", "lsp" },
--       filetypes = { "aql" },
--       root_dir  = lspconfig.util.root_pattern("aql.jsonic", ".git"),
--       settings  = {},
--     },
--   }
-- end
--
-- lspconfig.aql.setup({})
