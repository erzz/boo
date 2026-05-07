// Describes a Ghostty window's structure for layout capture.
//
// Reads {"windowId": "..."} from stdin. Returns:
//   {
//     "tabs": [
//       { "name": "...", "terminals": [
//           { "id": "...", "workingDirectory": "...", "title": "..." },
//           ...
//         ]
//       },
//       ...
//     ]
//   }
//
// or {"error": "..."}.
//
// What Ghostty's AppleScript dictionary exposes per terminal: id, title,
// working directory. It does NOT expose split direction, the command that
// originally launched the surface, or the environment. boo's `save` command
// is responsible for surfacing that limitation to the user.
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
      if (String(w.id()) !== target) continue;

      const tabs = [];
      const ws = w.tabs;
      for (let t = 0; t < ws.length; t++) {
        const tab = ws[t];
        const terms = [];
        const ts = tab.terminals;
        for (let k = 0; k < ts.length; k++) {
          const term = ts[k];
          terms.push({
            id: String(term.id()),
            workingDirectory: String(term.workingDirectory() || ""),
            title: String(term.name() || ""),
          });
        }
        tabs.push({
          name: String(tab.name() || ""),
          terminals: terms,
        });
      }
      return JSON.stringify({ tabs: tabs });
    }
    return JSON.stringify({ error: "window not found: " + target });
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
