# PinQuake

## UI (ui/)
- Package manager: pnpm
- Typecheck: `cd ui && pnpm typecheck`
- Lint: `cd ui && pnpm lint`
- Test: `cd ui && pnpm test`
- Build: `cd ui && pnpm build`

## Go backend
- Build: `go build ./...`
- Lint: `golangci-lint run`
- Use `go doc` to explore Go types/interfaces, not grep on module cache
- Dev server port: 8091

## OpenAPI codegen
- `openapi.json` and `ui/src/lib/api.generated.ts` are generated automatically by air on every rebuild — do not edit manually
- Manual regeneration: `go run . -openapi > openapi.json && cd ui && pnpm gen:api`
