# macOS auxiliary window design QA

- Source visual truth: Settings `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-a3627999-61a3-44ca-bf56-4e3cf395202f.png`; Workflows `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-7e9b5654-ae8f-4c70-9d23-ff27d2431932.png`
- Implementation screenshots: Settings `/Users/ityike/.codex/visualizations/2026/08/30/01a05089-d7e4-7242-8da5-71581b724b54/settings-macos-centered-clean.jpg`; Workflows `/Users/ityike/.codex/visualizations/2026/08/30/01a05089-d7e4-7242-8da5-71581b724b54/workflow-macos-current.png`
- Combined comparisons: Settings `/Users/ityike/.codex/visualizations/2026/08/30/01a05089-d7e4-7242-8da5-71581b724b54/settings-macos-centered-comparison.png`; Workflows `/Users/ityike/.codex/visualizations/2026/08/30/01a05089-d7e4-7242-8da5-71581b724b54/workflow-macos-comparison.png`
- Viewports: Settings 959 × 799 CSS pixels; Workflows 1039 × 759 CSS pixels
- Pixel dimensions and density: Settings source 1918 × 1598 at 2×, normalized to 959 × 799; implementation 959 × 799 at 1×. Workflows source 2078 × 1518 at 2×, normalized to 1039 × 759; implementation 1039 × 759 at 1×.
- State: macOS light appearance; Settings Appearance section selected; Workflows detail view selected
- Primary interactions tested: loaded Settings, switched from Appearance to Terminal, and returned to Appearance; loaded the Workflows detail window
- Console errors: none

## Full-view comparison evidence

The normalized Settings comparison shows “设置” centred in the full-width top drag strip, matching the title treatment already used by Workflows. The macOS title strip no longer paints a 216 × 52 rectangular sidebar surface: the computed background is transparent and the native inset sidebar remains the only coloured panel under the traffic lights. The redundant “偏好设置” heading remains absent.

The Workflows comparison continues to show “工作流” centred at the top of the full window, while Windows and Linux keep their compact sidebar title branch.

## Focused region evidence

The full-view comparison renders the title and traffic-light region at sufficient size, so an additional crop was not needed. Computed evidence for Settings confirms the title occupies the full 959px strip and centres at x = 479.5, the macOS sidebar-title overlay has a transparent background and no right border, the inset sidebar clip remains `inset(8px 4px 8px 8px round 16px)`, and no “偏好设置” element is rendered. Native traffic lights are not drawn by the browser preview, so their placement is taken from the source image.

## Findings

- No actionable P0/P1/P2 differences remain for the requested macOS Settings title placement or protruding sidebar-title background.
- Fonts and typography: the existing application font stack, 14px title size, semibold weight, and content hierarchy are preserved.
- Spacing and layout rhythm: the title is centred against the full window rather than the 216px sidebar; the rounded sidebar inset and 216px content grid remain unchanged.
- Colors and visual tokens: the macOS title overlay is transparent; the sidebar material retains the existing sidebar token. Windows/Linux still apply `var(--sidebar)` to their compact title segment.
- Image quality and asset fidelity: no image, icon, or raster asset was added or replaced.
- Copy and content: “设置” remains the window identity and “偏好设置” is absent.

## Comparison history

1. Earlier macOS Settings implementation kept “设置” in the compact left sidebar title region and painted a rectangular 216 × 52 sidebar-coloured overlay above the inset rounded native panel. These were P2 visual mismatches called out in the annotated source.
2. Fix: reused the existing desktop-platform helper. Windows/Linux retain the compact left title and coloured sidebar segment; macOS renders the title in a full-window centred layer and leaves the sidebar title segment transparent.
3. Post-fix evidence: the revised 959 × 799 browser capture shows the centred title and clean rounded sidebar top. DOM inspection confirms title centre x = 479.5, transparent overlay background, 0px overlay border, and zero redundant preferences headings.

## Implementation checklist

- [x] Centre the Settings title on macOS, matching Workflows.
- [x] Remove the macOS rectangular sidebar-title background that protruded over the rounded material.
- [x] Preserve the inset rounded sidebar and existing Settings content layout.
- [x] Keep Windows/Linux compact auxiliary chrome unchanged.
- [x] Verify navigation, console output, targeted tests, and the production build.

final result: passed
