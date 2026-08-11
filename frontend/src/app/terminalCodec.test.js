import test from "node:test";
import assert from "node:assert/strict";
import { decodeTerminalData, encodeTerminalData } from "./terminalCodec.js";

test("terminal transport preserves ANSI and multibyte UTF-8", () => {
  const input = "\u001b[31m你好, terminal\u001b[0m\r\n";
  const decoded = decodeTerminalData(encodeTerminalData(input));
  assert.equal(new TextDecoder().decode(decoded), input);
});

test("terminal transport accepts control-key input", () => {
  const input = "\u0003\u001b[A\t";
  assert.deepEqual([...decodeTerminalData(encodeTerminalData(input))], [3, 27, 91, 65, 9]);
});
