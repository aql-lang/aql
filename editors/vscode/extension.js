// Minimal VS Code extension that spawns `boru lsp` on stdio and wires
// it up as a Language Server Protocol client for *.boru files.
//
// Build/install:
//   cd editors/vscode
//   npm install
//   npx vsce package
//   code --install-extension boru-0.1.0.vsix

const { workspace } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const cfg = workspace.getConfiguration("boru");
  const serverPath = cfg.get("serverPath", "boru");

  const serverOptions = {
    command: serverPath,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  const clientOptions = {
    documentSelector: [
      { scheme: "file", language: "boru" },
    ],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.boru"),
    },
  };

  client = new LanguageClient(
    "boruLsp",
    "BORU Language Server",
    serverOptions,
    clientOptions,
  );

  client.start();
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
