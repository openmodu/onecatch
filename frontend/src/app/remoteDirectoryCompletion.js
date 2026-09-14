export function commonDirectoryPrefix(paths) {
  if (!paths.length) return "";
  let prefix = paths[0];
  for (const path of paths.slice(1)) {
    while (!path.startsWith(prefix)) prefix = prefix.slice(0, -1);
  }
  return prefix;
}

export function directoryQuery(value) {
  if (value === "~") return { directory: "~/", prefix: "" };
  const slash = value.lastIndexOf("/");
  return { directory: slash < 0 ? "" : value.slice(0, slash + 1), prefix: value.slice(slash + 1) };
}

// Cache whole directory listings, not individual typed prefixes. Concurrent
// keystrokes share a request, and backspacing can reveal previously hidden rows.
export function createDirectoryCompletionClient(load, now = Date.now) {
  const cache = new Map();
  return {
    hasDirectory(value) {
      const entry = cache.get(directoryQuery(value).directory);
      return Boolean(entry?.paths && now() - entry.time <= 30_000);
    },
    async complete(value) {
      const { directory, prefix } = directoryQuery(value);
      let entry = cache.get(directory);
      if (!entry || (entry.paths && now() - entry.time > 30_000)) {
        entry = { time: now() };
        cache.set(directory, entry);
        entry.pending = Promise.resolve().then(() => load(directory)).then((paths) => {
          entry.paths = paths;
          entry.time = now();
          return paths;
        }).catch((error) => {
          if (cache.get(directory) === entry) cache.delete(directory);
          throw error;
        });
        if (cache.size > 64) cache.delete(cache.keys().next().value);
      }
      const paths = entry.paths || await entry.pending;
      return paths.filter((path) => path.slice(0, -1).split("/").at(-1).startsWith(prefix));
    },
  };
}
