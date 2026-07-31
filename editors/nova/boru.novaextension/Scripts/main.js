//
// boru extension for Nova — starts the bundled `boru lsp` stdio language
// server and wires it up as a LanguageClient for *.boru files.
//
// The server binary is discovered from the "boru.lsp.path" configuration
// (defaults to `boru` on $PATH). Toggling "boru.lsp.enabled" or changing
// the path restarts the client live.
//

let client = null;

// Resolve the configured `boru` binary path, expanding a leading `~` to
// the user's home directory. Falls back to `boru` on $PATH.
function resolveServerPath() {
    let path = nova.config.get("boru.lsp.path", "string");
    if (!path || path.trim() === "") {
        path = "boru";
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

    if (nova.config.get("boru.lsp.enabled", "boolean") === false) {
        return;
    }

    const serverPath = resolveServerPath();

    // Server options: launch `boru lsp` and talk LSP over stdio.
    const serverOptions = {
        path: serverPath,
        args: ["lsp"],
        type: "stdio",
    };

    // Client options: attach to the boru syntax and watch *.boru files.
    const clientOptions = {
        syntaxes: ["boru"],
        debug: nova.inDevMode(),
    };

    client = new LanguageClient(
        "boru-lsp",
        "boru Language Server",
        serverOptions,
        clientOptions
    );

    try {
        // Surface server crashes as a notice rather than failing silently.
        client.onDidStop((err) => {
            if (err) {
                console.error("boru language server stopped:", err.message);
            }
        });

        client.start();

        // Dispose the client when the extension deactivates.
        nova.subscriptions.add(client);
    } catch (err) {
        if (nova.inDevMode()) {
            console.error("Failed to start boru language server:", err);
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
    nova.config.onDidChange("boru.lsp.path", startClient);
    nova.config.onDidChange("boru.lsp.enabled", startClient);
};

exports.deactivate = function () {
    stopClient();
};
