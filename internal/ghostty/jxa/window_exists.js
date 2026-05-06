// Looks up a Ghostty window by its (process-lifetime) stable ID.
// Reads {"windowId": "..."} from stdin.
// Returns {"exists": true|false} on success or {"error": "..."}.
(() => {
  try {
    ObjC.import('Foundation');
    const handle = $.NSFileHandle.fileHandleWithStandardInput;
    const data = handle.readDataToEndOfFile;
    const raw = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js || "";
    const params = raw.trim().length ? JSON.parse(raw) : {};
    const target = String(params.windowId || "");
    if (!target) return JSON.stringify({ error: "windowId is required" });

    const app = Application("Ghostty");
    const wins = app.windows;
    for (let i = 0; i < wins.length; i++) {
      if (String(wins[i].id()) === target) {
        return JSON.stringify({ exists: true });
      }
    }
    return JSON.stringify({ exists: false });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
