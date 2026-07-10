" AQL Language Server for classic Vim via prabirshrestha/vim-lsp.
"
" Prerequisites:
"   Plug 'prabirshrestha/async.vim'
"   Plug 'prabirshrestha/vim-lsp'
"
" Drop the block below into ~/.vimrc (or a file sourced from it).

" Map *.aql to the "aql" filetype so the LSP attaches.
autocmd BufRead,BufNewFile *.aql set filetype=aql

" Register the server. vim-lsp will spawn `aql lsp` and speak LSP
" over stdio. root_uri keeps server state per-project.
if executable('aql')
  augroup aql_lsp
    autocmd!
    autocmd User lsp_setup call lsp#register_server({
          \ 'name': 'aql-lsp',
          \ 'cmd': {server_info -> ['aql', 'lsp']},
          \ 'allowlist': ['aql'],
          \ 'root_uri': {server_info ->
          \   lsp#utils#path_to_uri(
          \     lsp#utils#find_nearest_parent_file_directory(
          \       lsp#utils#get_buffer_path(),
          \       ['aql.jsonic', '.git']))},
          \ })
  augroup END
endif

" Optional: convenience mappings for common LSP actions.
" autocmd FileType aql nmap <buffer> gd <plug>(lsp-definition)
" autocmd FileType aql nmap <buffer> K  <plug>(lsp-hover)
" autocmd FileType aql nmap <buffer> ]g <plug>(lsp-next-diagnostic)
" autocmd FileType aql nmap <buffer> [g <plug>(lsp-previous-diagnostic)
" autocmd FileType aql nmap <buffer> =  <plug>(lsp-document-format)
