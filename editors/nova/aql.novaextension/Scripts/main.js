//
// AQL extension for Nova — starts the bundled `aql lsp` stdio language
// server and wires it up as a LanguageClient for *.aql files.
//
// The server binary is discovered from the "aql.lsp.path" configuration
// (defaults to `aql` on $PATH). Toggling "aql.lsp.enabled" or changing
// the path restarts the client live.
//

let client = null;

// Resolve the configured `aql` binary path, expanding a leading `~` to
// the user's home directory. Falls back to `aql` on $PATH.
function resolveServerPath() {
    let path = nova.config.get("aql.lsp.path", "string");
    if (!path || path.trim() === "") {
        path = "aql";
    }
    if (path.startsWith("~/")) {
        const home = nova.path.expanduser("~");
        path = nova.path.join(home, path.slice(2));
    } else if (path === "~") {
        path = nova.path.expanduser("~");
    }
    return path;
}

function startClient() {
    // Never run two clients at once.
    stopClient();

    if (nova.config.get("aql.lsp.enabled", "boolean") === false) {
        return;
    }

    const serverPath = resolveServerPath();

    // Server options: launch `aql lsp` and talk LSP over stdio.
    const serverOptions = {
        path: serverPath,
        args: ["lsp"],
        type: "stdio",
    };

    // Client options: attach to the AQL syntax and watch *.aql files.
    const clientOptions = {
        syntaxes: ["aql"],
        debug: nova.inDevMode(),
    };

    client = new LanguageClient(
        "aql-lsp",
        "AQL Language Server",
        serverOptions,
        clientOptions
    );

    try {
        // Surface server crashes as a notice rather than failing silently.
        client.onDidStop((err) => {
            if (err) {
                console.error("AQL language server stopped:", err.message);
            }
        });

        client.start();

        // Dispose the client when the extension deactivates.
        nova.subscriptions.add(client);
    } catch (err) {
        if (nova.inDevMode()) {
            console.error("Failed to start AQL language server:", err);
        }
        client = null;
    }
}

function stopClient() {
    if (client) {
        client.stop();
        nova.subscriptions.remove(client);
        client = null;
    }
}

exports.activate = function () {
    startClient();

    // Restart the client when the relevant configuration changes so the
    // user does not have to reload the extension by hand.
    nova.config.onDidChange("aql.lsp.path", startClient);
    nova.config.onDidChange("aql.lsp.enabled", startClient);
};

exports.deactivate = function () {
    stopClient();
};
