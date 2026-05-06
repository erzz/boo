// Activates (focuses + brings to front) a Ghostty window by stable ID.
// Reads {"windowId": "..."} from stdin.
// Returns {"ok": true} or {"error": "..."}.
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
        app.activateWindow(wins[i]);
        return JSON.stringify({ ok: true });
      }
    }
    return JSON.stringify({ error: "window not found: " + target });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
