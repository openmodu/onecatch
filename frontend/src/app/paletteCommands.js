const HISTORY_KEY = "onecatch.commandHistory.v1";
const COMMANDS = [
  { id: "new-task", labelKey: "task.newTask", aliases: "new task create conversation 新建任务 新建对话 xjrw", shortcut: "N" },
  { id: "add-workspace", labelKey: "sidebar.openFolder", aliases: "open add folder workspace project 打开文件夹 添加项目 dkwjj tjxm", shortcut: "O" },
  { id: "templates", labelKey: "sidebar.templates", aliases: "templates prompts snippets 模板 提示词 快捷指令 muban mb" },
  { id: "skills", labelKey: "sidebar.skills", aliases: "skills extensions 技能 jineng jn" },
  { id: "usage", labelKey: "sidebar.usage", aliases: "usage tokens quota 用量 额度 消耗 yongliang yl" },
  { id: "settings", labelKey: "sidebar.settings", aliases: "settings preferences configuration 设置 配置 shezhi sz", shortcut: "," },
];

function scoreText(value, query) {
  if (value === query) return 100;
  if (value.startsWith(query)) return 80;
  if (value.includes(query)) return 60;
  // Short Latin abbreviations can skip letters; arbitrary long input should not.
  if (!/^[a-z]{2,6}$/.test(query)) return 0;
  let position = 0;
  for (const character of query) {
    position = value.indexOf(character, position);
    if (position < 0) return 0;
    position += 1;
  }
  return 20;
}

export function paletteCommandResults(t, query = "", history = []) {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  return COMMANDS.map((command, index) => {
    const label = t(command.labelKey);
    const fields = [label.toLocaleLowerCase(), ...command.aliases.split(" ")];
    const scores = terms.map((term) => Math.max(...fields.map((field) => scoreText(field, term))));
    const recentIndex = history.indexOf(command.id);
    return { ...command, label, index, score: scores.reduce((sum, score) => sum + score, 0), matches: scores.every(Boolean), recentIndex };
  }).filter((command) => command.matches).sort((a, b) =>
    b.score - a.score || (a.recentIndex < 0 ? Infinity : a.recentIndex) - (b.recentIndex < 0 ? Infinity : b.recentIndex) || a.index - b.index
  );
}

export function readCommandHistory(storage) {
  try {
    const data = JSON.parse(storage.getItem(HISTORY_KEY) || "[]");
    return Array.isArray(data) ? [...new Set(data.filter((id) => COMMANDS.some((command) => command.id === id)))].slice(0, COMMANDS.length) : [];
  } catch { return []; }
}

export function rememberCommand(storage, id) {
  if (!COMMANDS.some((command) => command.id === id)) return;
  try { storage.setItem(HISTORY_KEY, JSON.stringify([id, ...readCommandHistory(storage).filter((entry) => entry !== id)].slice(0, COMMANDS.length))); } catch { /* Navigation remains available when storage is disabled. */ }
}
