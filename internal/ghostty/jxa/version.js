// Returns the running Ghostty's version as JSON: {"version": "1.3.1"}
// or {"error": "..."} on failure. Read-only; does not require Automation permission.
(() => {
  try {
    const app = Application("Ghostty");
    return JSON.stringify({ version: app.version() });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
