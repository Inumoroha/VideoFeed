## Why

VideoFeed already has product, architecture, engineering, UI/UX, optimization, and module documents, but the information was spread across overlapping files. This change establishes a clean documentation baseline and records the long-lived platform rules in OpenSpec.

## What Changes

- Consolidate README into a project entry with startup commands and documentation map.
- Make `docs/product.md` the single product scope and feature status source.
- Merge engineering standards into `docs/engineering.md`.
- Consolidate the existing performance topic in `docs/optimization.md`.
- Normalize module documents under `docs/modules/`.
- Add `openspec/project.md` and a platform-basics capability spec.

## Capabilities

### New Capabilities

- `platform-basics`: Project-level documentation, engineering, module, and OpenSpec baseline for future VideoFeed changes.

### Modified Capabilities

None.

## Impact

- Affected documentation: `README.md`, `docs/product.md`, `docs/quickread.md`, `docs/uiux.md`, `docs/modules/*.md`, `docs/engineering.md`, `docs/optimization.md`.
- Remove `docs/modules/standard.md`; retain `docs/standard.md` only as a compatibility link. Consolidate the existing optimization document.
- Affected OpenSpec files: `openspec/project.md`, `openspec/changes/fill-platform-basics/*`.
- No application runtime behavior changes.
