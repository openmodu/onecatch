// Window-local drafts survive composer unmounts without writing prompts or
// attachment paths to persistent browser storage.
export function draftKey(workspaceID, sessionID, field) {
  return JSON.stringify([workspaceID, sessionID, field]);
}

export function createComposerDrafts() {
  const values = new Map();
  const listeners = new Map();
  return {
    get(key, fallback) {
      return values.has(key) ? values.get(key) : fallback;
    },
    set(key, update, fallback) {
      const current = values.has(key) ? values.get(key) : fallback;
      const next = typeof update === "function" ? update(current) : update;
      if (Object.is(current, next)) return;
      values.set(key, next);
      listeners.get(key)?.forEach((listener) => listener());
    },
    subscribe(key, listener) {
      if (!listeners.has(key)) listeners.set(key, new Set());
      const subscribers = listeners.get(key);
      subscribers.add(listener);
      return () => {
        subscribers.delete(listener);
        if (!subscribers.size) listeners.delete(key);
      };
    },
  };
}
