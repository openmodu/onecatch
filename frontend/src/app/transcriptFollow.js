// Layout changes can emit scroll events without the reader scrolling. Keep
// following across keyboard/composer resizes, but respect a move into history.
export function createTranscriptFollower(element) {
  let following = true;
  let last = measure();
  function measure() {
    return { top: element.scrollTop, height: element.scrollHeight, viewport: element.clientHeight };
  }
  return {
    pause() { following = false; last = measure(); },
    scroll() {
      const next = measure();
      const atBottom = next.height - next.top - next.viewport < 90;
      const resized = next.height !== last.height || next.viewport !== last.viewport;
      if (atBottom) following = true;
      else if (!resized && next.top < last.top) following = false;
      last = next;
    },
    sync(force = false) {
      if (force) following = true;
      if (following) element.scrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
      last = measure();
    },
  };
}
