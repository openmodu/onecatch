// Native code proxies local paths through the paired worker; the WebView never
// receives its token or tries to open the desktop's file:// URLs itself.
export function conversationImageURL(source, runID) {
  const value = String(source || "").trim();
  if (!value || !runID || /[\u0000-\u001f]/.test(value)) return "";
  if (/^https?:\/\//i.test(value)) {
    try {
      const url = new URL(value);
      return url.username || url.password ? "" : url.href;
    } catch { return ""; }
  }
  if (value.startsWith("//") || value.startsWith("#")) return "";
  if (/^[a-z][a-z0-9+.-]*:/i.test(value) && !/^file:\//i.test(value)) return "";
  return `/conversation-image?run=${encodeURIComponent(runID)}&path=${encodeURIComponent(value)}`;
}

// Rewrite before Streamdown's URL hardening, which otherwise discards relative
// paths and file: URLs before our image component ever sees them.
export function remarkConversationImages({ runID }) {
  return (tree) => {
    const definitions = new Map();
    const visit = (node, callback) => { callback(node); for (const child of node.children || []) visit(child, callback); };
    visit(tree, (node) => { if (node.type === "definition") definitions.set(node.identifier, node); });
    visit(tree, (node) => {
      if (node.type === "imageReference") {
        const definition = definitions.get(node.identifier);
        if (!definition) return;
        node.type = "image";
        node.url = definition.url;
        node.title = definition.title;
      }
      if (node.type === "image") node.url = conversationImageURL(node.url, runID);
    });
  };
}
