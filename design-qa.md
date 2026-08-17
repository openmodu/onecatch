# Design QA

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-6bab2bc9-8924-48dd-a225-a6760678ca94.png`, showing the new-task composer squeezed by an expanded inspector until the execution control wrapped onto a second row.
- Browser-rendered implementation: `http://127.0.0.1:4173/`
- Browser state: light theme, 1280 × 720 viewport, new-task screen with the inspector expanded to the maximum allowed width.

## Findings

No actionable P0, P1, or P2 differences remain in the requested scope.

- Composer consistency: new-task and existing-session composers now share the same control order and compact pill treatment.
- Existing Workflow session: attachment, Workflow, persisted permission, runtime profile, and execution actions stay on one baseline at desktop widths.
- Existing direct-Agent session: the execution target appears once as `Agent · runtime`; a runtime-profile control only appears when the selected runtime actually supports model, reasoning, or speed configuration.
- Duplicate removal: `modu_code` no longer renders as both the execution target and a redundant runtime-profile pill.
- Alignment: the execution target and permission remain at the left edge of the toolbar, while runtime profile and stop/resume/continue actions remain grouped on the right without a dead middle control slot.
- Minimum conversation width: inspector resizing stops once the conversation reaches 620 px, preventing the side panel from consuming the composer's usable control area.
- Narrow-container behavior: at the minimum width, the toolbar switches to compact Agent, permission, runtime, and icon-only execution controls while preserving a single row.
- Resize recovery: a `ResizeObserver` re-clamps the saved inspector width when the window or sidebar changes size, so a formerly valid inspector width cannot squeeze the conversation after a resize.
- Execution target: every new task selects exactly one target—either a coding Agent or an orchestrated Workflow. `Agent · Codex` maps internally to the single-Agent runner without exposing `单 Agent 完成` as a second user-facing workflow choice.
- Compact selection UI: the trigger and menu now size to their content, the redundant “Agent 与工作流二选一” heading is removed, and Agent entries no longer repeat identical explanatory text.
- Permission control: read-only, workspace access, and full access are explicit task choices. Full access stays disabled unless both application security and the workspace allow it.
- Direct Agent: selecting Codex, Claude Code, or modu_code exposes the matching model/runtime controls; selecting an orchestrated Workflow hides those per-Agent overrides.
- Execution semantics: the selected permission is persisted with the task, applied directly to single-Agent runs, and acts as a least-privilege cap across orchestrated workflow steps.

## Interaction and Accessibility Checks

- The execution-target trigger has the accessible name “选择 Agent 或工作流” and presents mutually exclusive Agent and Workflow sections.
- Mutual exclusion is communicated by the single checked radio item across both grouped sections; no instructional “二选一” sentence is shown.
- Agent, Workflow, and permission entries use radio-menu semantics and visibly identify the active choice.
- Existing Workflow sessions keep their Workflow target and persisted permission read-only while allowing the active runtime profile to remain visible where configurable.
- Paused and completed direct-Agent sessions keep the Agent selector available for subsequent turns; running sessions present it read-only.
- The permission menu explains each access level; full access exposes a proper disabled state in a workspace that cannot grant it.
- Attachment, immediate/queued execution, status inspector, and keyboard submission remain available.

## Verification

- Focused frontend regressions: existing-session Agent/Workflow, permission, completed-conversation, and new-task execution-target assertions pass.
- Runtime selection unit tests: 9 passed, including configurable-profile visibility for Codex/Claude Code and profile suppression for `modu_code`.
- Production frontend build: passed.
- Browser QA: inspector expanded by keyboard to its maximum width; conversation measured exactly 620 px, inspector stopped at 436 px, and all five toolbar controls remained on the same vertical baseline.
- Static diff validation: `git diff --check` passed.

## Implementation Checklist

- [x] Move the new-task composer into the central working area.
- [x] Require exactly one Agent or Workflow target for each new task.
- [x] Keep the internal `single_agent` runner out of the user-facing Workflow list.
- [x] Add task-level read-only, workspace, and full-access controls.
- [x] Persist and enforce the selected permission in backend run resolution.
- [x] Verify Workflow and permission switching in the live browser.
- [x] Keep all task composer controls on one desktop row without clipping.
- [x] Remove redundant execution-target copy and compact the trigger/menu width.
- [x] Reuse the new-task control layout in existing conversations.
- [x] Remove the duplicated `modu_code` runtime control.
- [x] Preserve paused/completed direct-Agent switching for follow-up turns.
- [x] Preserve a 620 px minimum conversation width while resizing the inspector.
- [x] Keep the new-task controls on one row at that minimum width.

final result: passed
