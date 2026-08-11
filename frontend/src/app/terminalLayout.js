export const paneNode = (paneID) => ({ type: "pane", paneID });

export function clampSplitRatio(ratio, availablePixels, minimumPixels) {
  if (!Number.isFinite(availablePixels) || availablePixels <= 0) return 0.5;
  const minimumRatio = Math.min(0.45, minimumPixels / availablePixels);
  return Math.max(minimumRatio, Math.min(1 - minimumRatio, ratio));
}

export function splitPane(node, paneID, nextPaneID, direction, splitID) {
  if (!node) return node;
  if (node.type === "pane") {
    return node.paneID === paneID
      ? { type: "split", id: splitID, direction, ratio: 0.5, first: node, second: paneNode(nextPaneID) }
      : node;
  }
  return { ...node, first: splitPane(node.first, paneID, nextPaneID, direction, splitID), second: splitPane(node.second, paneID, nextPaneID, direction, splitID) };
}

export function removePane(node, paneID) {
  if (!node) return null;
  if (node.type === "pane") return node.paneID === paneID ? null : node;
  const first = removePane(node.first, paneID);
  const second = removePane(node.second, paneID);
  if (!first) return second;
  if (!second) return first;
  return { ...node, first, second };
}

export function updateSplitRatio(node, splitID, ratio) {
  if (!node || node.type === "pane") return node;
  if (node.id === splitID) return { ...node, ratio: Math.max(0.12, Math.min(0.88, ratio)) };
  return { ...node, first: updateSplitRatio(node.first, splitID, ratio), second: updateSplitRatio(node.second, splitID, ratio) };
}

export function paneIDs(node) {
  if (!node) return [];
  if (node.type === "pane") return [node.paneID];
  return [...paneIDs(node.first), ...paneIDs(node.second)];
}

export function layoutGeometry(node, rect = { x: 0, y: 0, width: 1, height: 1 }, result = { panes: [], splits: [] }) {
  if (!node) return result;
  if (node.type === "pane") {
    result.panes.push({ paneID: node.paneID, rect });
    return result;
  }
  result.splits.push({ id: node.id, direction: node.direction, ratio: node.ratio, rect });
  if (node.direction === "vertical") {
    layoutGeometry(node.first, { ...rect, width: rect.width * node.ratio }, result);
    layoutGeometry(node.second, { ...rect, x: rect.x + rect.width * node.ratio, width: rect.width * (1 - node.ratio) }, result);
  } else {
    layoutGeometry(node.first, { ...rect, height: rect.height * node.ratio }, result);
    layoutGeometry(node.second, { ...rect, y: rect.y + rect.height * node.ratio, height: rect.height * (1 - node.ratio) }, result);
  }
  return result;
}
