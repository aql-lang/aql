# VS Code AQL extension

Minimal Language Server client that runs `aql lsp` on stdio for any
`*.aql` file.

## Install (local development)

```sh
cd cmd/go/internal/lsp/editors/vscode
npm install
npx --yes @vscode/vsce package          # writes aql-0.1.0.vsix
code --install-extension aql-0.1.0.vsix
```

## Configure

Settings → `AQL: Server Path` to point at a non-default `aql` binary,
or set in `settings.json`:

```json
{
  "aql.serverPath": "/usr/local/bin/aql"
}
```

## Develop

Press `F5` from this directory in VS Code to launch an Extension
Development Host with the extension live-loaded.
