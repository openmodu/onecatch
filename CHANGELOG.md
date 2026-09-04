# Changelog

The first version section is the source of truth for all OneCatch builds and
release artifacts. Add a new section here before creating a release tag.

## 0.2.1

- Let embedded Modu runs pause for human input with multi-question cards, suggested choices, custom answers, and an explicit skip path.

## 0.2.0

- Add a Skills workspace backed by `~/.onecatch/skills`, with an adaptive card library and create, edit, inspect, and delete workflows.
- Debug individual skills through Modu with streamed output, cancellation, and persisted run history.
- Discover Codex, Claude Code, Modu Code, and custom skill directories, then sync selected skills with rsync while tracking per-target metadata and status.

## 0.1.13

- Stop double-clicks on the macOS review toolbar and other content areas from resizing the window; let Wails handle double-clicks only in marked titlebar regions.
- Upgrade the Wails framework, project CLI, and JavaScript runtime to v3.0.0-beta.16 and enforce matching runtime versions.
- Update the embedded Modu Code runtime to commit cc1462c and refresh its required Go dependencies.

## 0.1.12

- Syntax-highlight fenced code blocks in agent responses with a language label toolbar, one-click copy, and a per-block wrap toggle.
- Map common fence-language aliases to Prism grammars while falling back to escaped plain text for unknown or untrusted language names.
- Read the live code text when copying (including the latest streamed content) and exclude toolbar labels from the clipboard, with fallback to the Web Clipboard API and clear copy/error feedback.
- Keep token colors theme-aware across light and dark modes.

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
