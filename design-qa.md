# Design QA

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-7a8dca3c-44fc-492d-8a7d-6a3747bc9cc6.png`, with the user's explicit correction that the Agent/Workflow relationship should be conveyed through interaction instead of explanatory copy.
- Browser-rendered implementation: `http://127.0.0.1:4173/`
- Browser state: light theme, 1280 × 720 viewport, new-task screen with the right inspector open.

## Findings

No actionable P0, P1, or P2 differences remain in the requested scope.

- Composer placement: the title and input card now occupy the central working area instead of sitting against the bottom edge.
- Single-row controls: attachment, Workflow, permission, Harness, runtime profile, and execution stay on one baseline at desktop widths.
- Execution target: every new task selects exactly one target—either a coding Agent or an orchestrated Workflow. `Agent · Codex` maps internally to the single-Agent runner without exposing `单 Agent 完成` as a second user-facing workflow choice.
- Compact selection UI: the trigger and menu now size to their content, the redundant “Agent 与工作流二选一” heading is removed, and Agent entries no longer repeat identical explanatory text.
- Permission control: read-only, workspace access, and full access are explicit task choices. Full access stays disabled unless both application security and the workspace allow it.
- Direct Agent: selecting Codex, Claude Code, or modu_code exposes the matching model/runtime controls; selecting an orchestrated Workflow hides those per-Agent overrides.
- Execution semantics: the selected permission is persisted with the task, applied directly to single-Agent runs, and acts as a least-privilege cap across orchestrated workflow steps.

## Interaction and Accessibility Checks

- The execution-target trigger has the accessible name “选择 Agent 或工作流” and presents mutually exclusive Agent and Workflow sections.
- Mutual exclusion is communicated by the single checked radio item across both grouped sections; no instructional “二选一” sentence is shown.
- Agent, Workflow, and permission entries use radio-menu semantics and visibly identify the active choice.
- Selecting an orchestrated Workflow immediately removes the Harness and runtime-profile controls.
- The permission menu explains each access level; full access exposes a proper disabled state in a workspace that cannot grant it.
- Attachment, immediate/queued execution, status inspector, and keyboard submission remain available.

## Verification

- Focused frontend regressions: the new-task workflow and permission assertions pass.
- Runtime selection unit tests: passed.
- Focused backend packages: `go test ./internal/domain/tasks ./internal/service/desktop ./internal/repo/tasks` passed.
- Browser QA: Agent/Workflow mutual exclusion, Codex runtime visibility, orchestrated Workflow switching, permission menu, full-access disabled state, and single-row desktop composition verified with the inspector both open and collapsed.
- The full static frontend file still reports the 7 pre-existing Sidebar/Command Palette baseline assertions; the new-task tests pass.

## Implementation Checklist

- [x] Move the new-task composer into the central working area.
- [x] Require exactly one Agent or Workflow target for each new task.
- [x] Keep the internal `single_agent` runner out of the user-facing Workflow list.
- [x] Add task-level read-only, workspace, and full-access controls.
- [x] Persist and enforce the selected permission in backend run resolution.
- [x] Verify Workflow and permission switching in the live browser.
- [x] Keep all task composer controls on one desktop row without clipping.
- [x] Remove redundant execution-target copy and compact the trigger/menu width.

final result: passed
