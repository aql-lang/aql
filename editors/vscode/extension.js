// Minimal VS Code extension that spawns `aql lsp` on stdio and wires
// it up as a Language Server Protocol client for *.aql files.
//
// Build/install:
//   cd cmd/go/internal/lsp/editors/vscode
//   npm install
//   npx vsce package
//   code --install-extension aql-0.1.0.vsix

const { workspace } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const cfg = workspace.getConfiguration("aql");
  const serverPath = cfg.get("serverPath", "aql");

  const serverOptions = {
    command: serverPath,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  const clientOptions = {
    documentSelector: [
      { scheme: "file", language: "aql" },
    ],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.aql"),
    },
  };

  client = new LanguageClient(
    "aqlLsp",
    "AQL Language Server",
    serverOptions,
    clientOptions,
  );

  client.start();
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
