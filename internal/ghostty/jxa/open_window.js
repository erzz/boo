// Opens a new Ghostty window using a surface configuration read as JSON
// from stdin. Returns {"windowId": "<id>"} on success or {"error": "..."}.
//
// Input shape (any field optional):
//   { "workingDirectory": "/abs/path",
//     "initialInput": "nvim .\n",
//     "env": { "FOO": "bar" } }
//
// Note: there is intentionally no "command" field. See open_layout.js
// and internal/cli/switch.go::splitToParams for the reasoning.
//
// Reading stdin in JXA: ObjC bridge to NSFileHandle.
(() => {
  try {
    ObjC.import('Foundation');
    const handle = $.NSFileHandle.fileHandleWithStandardInput;
    const data = handle.readDataToEndOfFile;
    const raw = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js || "";
    const params = raw.trim().length ? JSON.parse(raw) : {};

    const app = Application("Ghostty");
    app.includeStandardAdditions = true;

    // IMPORTANT: see open_layout.js — Ghostty silently ignores fields passed
    // via the constructor's object argument. Properties must be assigned to
    // the returned record after creation.
    const cfg = app.newSurfaceConfiguration();
    if (params.workingDirectory) cfg.initialWorkingDirectory = params.workingDirectory;
    // cfg.command is intentionally never set — see open_layout.js.
    if (params.initialInput)     cfg.initialInput = params.initialInput;
    if (params.env) {
      const envList = [];
      for (const k of Object.keys(params.env)) envList.push(`${k}=${params.env[k]}`);
      if (envList.length) cfg.environmentVariables = envList;
    }

    const win = app.newWindow({ withConfiguration: cfg });

    return JSON.stringify({ windowId: String(win.id()) });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
