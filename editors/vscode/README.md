# VS Code BORU extension

Minimal Language Server client that runs `boru lsp` on stdio for any
`*.boru` file.

## Install (local development)

```sh
cd editors/vscode
npm install
npx --yes @vscode/vsce package          # writes boru-0.1.0.vsix
code --install-extension boru-0.1.0.vsix
```

## Configure

Settings → `BORU: Server Path` to point at a non-default `boru` binary,
or set in `settings.json`:

```json
{
  "boru.serverPath": "/usr/local/bin/boru"
}
```

## Develop

Press `F5` from this directory in VS Code to launch an Extension
Development Host with the extension live-loaded.
