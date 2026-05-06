// Probes Automation permission by counting Ghostty's windows. This requires the
// same permission as creating windows but doesn't open any UI. Returns
// {"ok": true, "windowCount": N} on success or {"error": "..."} on failure.
(() => {
  try {
    const app = Application("Ghostty");
    const count = app.windows.length;
    return JSON.stringify({ ok: true, windowCount: count });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
