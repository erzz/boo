// Opens a new Ghostty window using a surface configuration read as JSON
// from stdin. Returns {"windowId": "<id>"} on success or {"error": "..."}.
//
// Input shape (any field optional):
//   { "workingDirectory": "/abs/path",
//     "command": "nvim .",
//     "env": { "FOO": "bar" } }
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

    const cfgFields = {};
    if (params.workingDirectory) cfgFields.initialWorkingDirectory = params.workingDirectory;
    if (params.command)          cfgFields.command = params.command;
    if (params.env) {
      const envList = [];
      for (const k of Object.keys(params.env)) envList.push(`${k}=${params.env[k]}`);
      if (envList.length) cfgFields.environmentVariables = envList;
    }

    const cfg = app.newSurfaceConfiguration(cfgFields);
    const win = app.newWindow({ withConfiguration: cfg });

    return JSON.stringify({ windowId: String(win.id()) });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
