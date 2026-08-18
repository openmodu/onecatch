export function shouldSubmitComposer(event, composing = false) {
  const nativeEvent = event.nativeEvent || event;
  return event.key === "Enter"
    && !event.shiftKey
    && !composing
    && !nativeEvent.isComposing
    && nativeEvent.keyCode !== 229;
}
