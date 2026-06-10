# Agent Marketplace Web Progress

## 2026-06-09

- Built a standalone React/Vite prototype for the selected option 3 workbench direction.
- Implemented simulated WeChat login, Agent selection, usage-count balance, buy-credits modal, per-use checkout, order filters, order detail, execution status, and delivery artifact preview.
- Billing model is usage-count based: one Agent execution consumes one use.
- Verification completed: `npm run build`.
- Visual browser QA: pending because Browser/Chrome capture tools were not exposed in this thread; Playwright requires explicit user approval under the Product Design browser policy.
- Adjustment: removed the extra far-right persistent order list column. The selected option 3 layout should keep the right rail focused on usage billing, order details, and delivery preview.
- Adjustment: changed the overall shell toward a Codex desktop-style three-pane layout: fixed left navigation/context pane, independently scrolling center work pane, and independently scrolling right inspector pane.
- Adjustment: made the right inspector pane collapsible. Expanded state shows billing/order/delivery details; collapsed state keeps a narrow clickable detail rail.
- Adjustment: moved authentication to the lower-left sidebar account area. The prototype now supports simulated WeChat authorization login and Google email login, with logout state handling.
- Adjustment: made the overall visual treatment more desktop-app-like: flatter pane backgrounds, tighter toolbar, denser navigation, reduced card elevation, compact panels, and tighter workbench spacing.
- Migration: moved prototype from `modu/docs/agent_marketplace_web` to `oneshot/design/prototype`.
- Documentation: added prototype README, development rules, and Go server + Wails v3 implementation notes.
