// Ignore late responses from an older refresh of the same worker. Each worker
// has its own generation, so switching machines cannot discard another cache.
export function createWorkspaceLoader(fetchItems, publish) {
  const generations = new Map();
  return async (workerID) => {
    const generation = (generations.get(workerID) || 0) + 1;
    generations.set(workerID, generation);
    try {
      const items = await fetchItems(workerID) || [];
      if (generations.get(workerID) !== generation) return null;
      publish(workerID, items);
      return items;
    } catch (error) {
      if (generations.get(workerID) !== generation) return null;
      throw error;
    }
  };
}
