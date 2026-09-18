import { useCallback, useRef, useSyncExternalStore } from "react";

export function useComposerDraft(store, key, fallback) {
  const target = useRef({ key, fallback });
  target.current = { key, fallback };
  const subscribe = useCallback((listener) => store.subscribe(key, listener), [store, key]);
  const snapshot = useCallback(() => store.get(key, fallback), [store, key, fallback]);
  // Event handlers in App are stable and always edit the visible draft.
  // Async work must capture its key and write to the store explicitly.
  const setValue = useCallback((update) => {
    store.set(target.current.key, update, target.current.fallback);
  }, [store]);
  return [useSyncExternalStore(subscribe, snapshot), setValue];
}
