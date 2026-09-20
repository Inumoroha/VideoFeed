## Context

The project had valuable documentation, but responsibilities overlapped. README contained detailed product tables, `docs/standard.md` and `docs/modules/standard.md` both described engineering rules, and the existing optimization topic mixed planned capabilities with implementation. The target repository had no OpenSpec baseline.

## Goals / Non-Goals

**Goals:**

- Establish a small set of clear documentation entry points.
- Preserve existing product, architecture, engineering, UI, and module knowledge.
- Make module documentation easier to scan and extend.
- Add an OpenSpec project context and a baseline capability spec.
- Keep the change limited to documentation and OpenSpec artifacts.

**Non-Goals:**

- Implement new backend or frontend functionality.
- Change API behavior.
- Change database schema.
- Commit or push repository changes.

## Decisions

- README is the project entry and links to focused documents.
- Product status lives in `docs/product.md`.
- Engineering standards live in `docs/engineering.md`.
- Feed performance and stability content lives in `docs/optimization.md`.
- Module docs use the same section pattern: responsibilities, APIs, data tables, business rules, tests, frontend touchpoints.
- OpenSpec `platform-basics` captures the documentation and engineering baseline as a long-lived capability.

## Risks / Trade-offs

- Some older detailed prose was compressed to reduce duplication.
- The consolidated engineering entry requires links to be updated; the old standard entry remains a compatibility link.
- Planned modules remain specification-level until implementation changes add code and tests.

## Target adaptation

- Map source account documentation to the existing user module.
- Preserve PostgreSQL, the root Go module, separate worker, existing Redis atomic operations, and RabbitMQ failure boundaries.
- Retain historical migration and integration evidence via `docs/migrations.md`.
- Route audit found six existing relation operations missing from OpenAPI; document them without changing application behavior.
