// Returns the ID of Ghostty's front (frontmost) window, or "" if there are
// no windows. Used by `boo save` (no args) to detect "the project I'm
// currently in".
//
// Output: {"windowId": "<id>"} on success, {"windowId": ""} when no window,
// {"error": "..."} on failure.
(() => {
  try {
    const app = Application("Ghostty");
    if (!app.running()) return JSON.stringify({ windowId: "" });
    // `frontWindow` is the sdef property; in JXA the camelCase form maps to
    // the AppleScript `front window` property. Returns null/undefined when
    // there are no windows.
    let win;
    try {
      win = app.frontWindow();
    } catch (_) {
      return JSON.stringify({ windowId: "" });
    }
    if (!win) return JSON.stringify({ windowId: "" });
    return JSON.stringify({ windowId: String(win.id()) });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
