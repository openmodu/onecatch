# Changelog

The first version section is the source of truth for all OneCatch builds and
release artifacts. Add a new section here before creating a release tag.

## 0.1.11

- Render agent thinking as prose in the run timeline instead of dressing it as a tool call, and label a thinking-only group as thinking rather than as zero tool calls.
- Restore Grok Build tool calls that disappeared from the run log when a session update carried its content as an array.

## 0.1.10

- Queue follow-up messages while an agent is running and promote any pending message to steer the active turn.
- Reduce the visual footprint of the restart-to-update control while preserving its accessible click target.

## 0.1.9

- Persist verified update packages across app restarts and revalidate them before installation.
- Improve remote workspace performance with persistent command channels, cached metadata, and safer filesystem mirroring across Codex, Claude Code, and Modu Code.
- Render streamed responses more smoothly with display-synchronized batching and a compact, theme-aware caret.

## 0.1.8

- 支持在输入框中输入 `$skill_name`，选择并指定当前 Agent 的技能。
- 统一桌面、iOS、Android、安装包和更新清单的版本及发布说明来源。

## 0.1.7

- Refined the available-update download action with a quieter icon and visual treatment.
