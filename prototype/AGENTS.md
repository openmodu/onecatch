# Prototype Instructions

Run the local server yourself and open the preview in the in-app browser. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

## Current Prototype Decisions

- Visual source: generated Product Design option 3, the usage-and-delivery mission-control workspace.
- Product model: vertical Agent marketplace with simulated WeChat login, usage-count billing, order tracking, and delivery artifact preview.
- Billing rule: charge by usage count. Agent execution consumes 1 use; users can buy additional uses.
- First implementation target: full interactive single-page prototype in this directory.
- Target production stack: Go server and Go/Wails v3 desktop app.
- Durable implementation notes live in `IMPLEMENTATION.md`; prototype development rules live in `DEVELOPMENT.md`.
