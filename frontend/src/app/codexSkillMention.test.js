import test from "node:test";
import assert from "node:assert/strict";
import {
  filterCodexSkills,
  findCodexSkillTrigger,
  insertCodexSkill,
  remarkCodexSkillMentions,
  splitCodexSkillMentions,
} from "./codexSkillMention.js";

test("opens a skill trigger at the beginning or after whitespace", () => {
  assert.deepEqual(findCodexSkillTrigger("$git"), { start: 0, caret: 4, query: "git" });
  assert.deepEqual(findCodexSkillTrigger("请使用 $git"), { start: 4, caret: 8, query: "git" });
  assert.deepEqual(findCodexSkillTrigger("第一行\n$doc"), { start: 4, caret: 8, query: "doc" });
});

test("does not open a skill trigger inside ordinary text", () => {
  assert.equal(findCodexSkillTrigger("price$usd"), null);
  assert.equal(findCodexSkillTrigger("$git commit"), null);
});

test("filters by invocation name before descriptions", () => {
  const skills = [
    { name: "documents", description: "Create files" },
    { name: "git-commit", description: "Create a clean commit" },
    { name: "other", description: "Git helper" },
  ];
  assert.deepEqual(filterCodexSkills(skills, "git").map((item) => item.name), ["git-commit", "other"]);
});

test("replaces the active trigger with a skill marker and trailing space", () => {
  const value = "请使用 $git 完成";
  const trigger = findCodexSkillTrigger(value, 8);
  assert.deepEqual(insertCodexSkill(value, trigger, "git-commit"), {
    value: "请使用 $git-commit 完成",
    caret: 16,
  });
});

test("splits valid skill mentions without treating currency or embedded dollars as skills", () => {
  assert.deepEqual(splitCodexSkillMentions("$git-commit 完成，价格 price$usd，另用 $a.stock:v2"), [
    { type: "skill", name: "git-commit", value: "$git-commit" },
    { type: "text", value: " 完成，价格 price$usd，另用 " },
    { type: "skill", name: "a.stock:v2", value: "$a.stock:v2" },
  ]);
});

test("remark plugin highlights prose mentions but leaves code and existing links alone", () => {
  const tree = {
    type: "root",
    children: [
      { type: "paragraph", children: [{ type: "text", value: "使用 $a-stock-data 查询" }] },
      { type: "inlineCode", value: "$git-commit" },
      { type: "link", url: "#docs", children: [{ type: "text", value: "$documents" }] },
    ],
  };
  remarkCodexSkillMentions()(tree);
  assert.deepEqual(tree.children[0].children, [
    { type: "text", value: "使用 " },
    { type: "link", url: "#onecatch-skill:a-stock-data", children: [{ type: "text", value: "$a-stock-data" }] },
    { type: "text", value: " 查询" },
  ]);
  assert.deepEqual(tree.children[1], { type: "inlineCode", value: "$git-commit" });
  assert.deepEqual(tree.children[2], { type: "link", url: "#docs", children: [{ type: "text", value: "$documents" }] });
});
