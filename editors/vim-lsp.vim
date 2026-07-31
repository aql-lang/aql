" BORU Language Server for classic Vim via prabirshrestha/vim-lsp.
"
" Prerequisites:
"   Plug 'prabirshrestha/async.vim'
"   Plug 'prabirshrestha/vim-lsp'
"
" Drop the block below into ~/.vimrc (or a file sourced from it).

" Map *.boru to the "boru" filetype so the LSP attaches.
autocmd BufRead,BufNewFile *.boru set filetype=boru

" Register the server. vim-lsp will spawn `boru lsp` and speak LSP
" over stdio. root_uri keeps server state per-project.
if executable('boru')
  augroup boru_lsp
    autocmd!
    autocmd User lsp_setup call lsp#register_server({
          \ 'name': 'boru-lsp',
          \ 'cmd': {server_info -> ['boru', 'lsp']},
          \ 'allowlist': ['boru'],
          \ 'root_uri': {server_info ->
          \   lsp#utils#path_to_uri(
          \     lsp#utils#find_nearest_parent_file_directory(
          \       lsp#utils#get_buffer_path(),
          \       ['boru.jsonic', '.git']))},
          \ })
  augroup END
endif

" Optional: convenience mappings for common LSP actions.
" autocmd FileType boru nmap <buffer> gd <plug>(lsp-definition)
" autocmd FileType boru nmap <buffer> K  <plug>(lsp-hover)
" autocmd FileType boru nmap <buffer> ]g <plug>(lsp-next-diagnostic)
" autocmd FileType boru nmap <buffer> [g <plug>(lsp-previous-diagnostic)
" autocmd FileType boru nmap <buffer> =  <plug>(lsp-document-format)
