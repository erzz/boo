// Closes a Ghostty window by stable ID.
// Reads {"windowId": "..."} from stdin.
// Returns {"ok": true} or {"error": "..."}.
//
// Safety: we re-verify the matched window's ID immediately before issuing
// closeWindow, and bail out if the lookup is inconsistent. JXA's element
// references can be evaluated lazily; a stale reference closing the wrong
// window has previously crashed a host shell. Defense in depth: the Go side
// also refuses to run integration tests from inside Ghostty.
//
// Note: Ghostty's "close window" command may prompt for confirmation if the
// user's setting is on. boo accepts whatever Ghostty does — we don't try to
// suppress the prompt.
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
      const w = wins[i];
      const wid = String(w.id());
      if (wid !== target) continue;
      // Re-verify the reference resolves to the same id we matched on; if
      // not, refuse rather than risk closing the wrong window.
      if (String(w.id()) !== target) {
        return JSON.stringify({ error: "window reference shifted between match and close: " + target });
      }
      app.closeWindow(w);
      return JSON.stringify({ ok: true });
    }
    // Already gone — treat as success.
    return JSON.stringify({ ok: true, alreadyGone: true });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
