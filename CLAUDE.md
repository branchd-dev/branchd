# Branchd

## What is Branchd?
Self-hosted database branching service for PostgreSQL databases.

## Components
- API server (Go, Gin, SQLite, OpenAPI): `cmd/server/main.go`
- Workers (asynq): `cmd/worker/main.go`
- Landing page(NextJS): `site`
- Admin dashboard UI (Vite, React, TypeScript, Tailwind, shadcn): `cmd/worker/main.go`

## Key details
- Tasks: `Makefile`
- [MUST USE] OpenAPI generated APIs: `web/src/lib/openapi.ts`, `web/src/hooks/use-api.ts`. Never use `fetch` for API requests.
- Cloudformation template for VM setup: `scripts/server_setup.sh`
- Models: `internal/models/models.go`
- API endpoints: `internal/server/server.go`
