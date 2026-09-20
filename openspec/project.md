# VideoFeed Project Context

## Purpose

VideoFeed is a short-video Feed system. It provides a practical engineering baseline for account, video publishing, Feed delivery, recommendation, interaction, relation, governance, monitoring, and Web client workflows.

## Technology

- Go API with Gin and GORM.
- PostgreSQL for durable data.
- Redis for Feed cache, hot ranking, counters, and short-lived state.
- RabbitMQ for asynchronous write-behind and fanout work.
- JWT for API authentication.
- React and Vite for the Web client.
- OpenSpec for change proposals and long-lived capability specifications.

## Architecture

Backend code lives at the repository root and follows four layers:

- `domain/{module}`: entities, business invariants, domain errors, repository interfaces.
- `application/{module}`: use cases, cursors, idempotency, cross-entity workflows.
- `infrastructure`: PostgreSQL, Redis, RabbitMQ, JWT, configuration, persistence models.
- `handler`, `router`, `middleware`: HTTP handlers, DTOs, middleware, and route registration.

The standard dependency assembly order is:

```text
Config -> DB/Redis/RabbitMQ/JWT -> Repository -> Service -> Handler -> Router
```

## Documentation Map

- `README.md`: project entry, startup commands, documentation map.
- `docs/product.md`: product scope, module map, P0/P1 status.
- `docs/quickread.md`: newcomer code-reading guide.
- `docs/architecture.md`: system diagrams and core flows.
- `docs/engineering.md`: engineering standards.
- `docs/optimization.md`: Feed performance and stability strategy.
- `docs/uiux.md`: Web UI/UX specification.
- `docs/modules/*.md`: module-level designs.

## Conventions

- New backend modules follow Domain -> Application -> Infrastructure -> Interfaces.
- New APIs use REST resource paths and cursor pagination for lists.
- Write APIs use `Idempotency-Key` when repeat submission is realistic.
- API tests live under `internal/router` and `internal/handler`; unit and repository tests live beside the corresponding Go packages.
- Web changes should preserve the current React/Vite structure unless page growth requires a split.
- Documentation updates are part of feature delivery.

## Verification

Use these checks when relevant:

```bash
npx --yes @fission-ai/openspec@1.2.0 validate --all --strict
go test ./...
npm --prefix web run build
```

## Target-specific constraints

- Go module: `github.com/inumoroha/video-feed`; user module name: `user`. Existing constructors include `NewService`, `NewHandler`, and `New`.
- Config, database and JWT live in `internal/config`, `internal/database`, and `internal/auth/jwt`.
- API: `cmd/server`; asynchronous consumers: `cmd/worker`; frontend: `web`.
- PostgreSQL tables include `users` and `videos`. Ports: API 9000, Web 5173, host PostgreSQL 5433.
- Follow `AGENTS.md`: Chinese comments immediately precede changed functions and structs; Chinese Conventional Commits only when requested.
- OpenAPI describes implemented Gin routes only. Planned modules are explicitly labeled; internal recommendation routes currently have no authentication.
- Run Go vet, race checks when applicable, Compose validation, OpenAPI YAML parsing and documentation link checks as described in `docs/engineering.md`.
