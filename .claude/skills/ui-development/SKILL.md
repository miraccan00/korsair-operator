# /ui-development

Guide for working on Korsair's React frontend (`frontend/`).

## Stack
- React 18, TypeScript, Vite, Tailwind CSS, React Router
- Built output embedded into Go binary via `cmd/web/main.go`
- API base: `/api/v1/` (proxied to operator API server at :8090)

## Dev Workflow
```
cd frontend
npm install
npm run dev     # Vite dev server with HMR
npm run build   # production build → cmd/web/static/dist/
npm run lint    # ESLint check
```

## Conventions
- Components in `frontend/src/components/` — one file per component.
- Pages in `frontend/src/pages/` — mapped to React Router routes.
- API calls via `frontend/src/api/` — typed fetch wrappers, never inline fetch.
- Tailwind only — no CSS modules or inline styles.
- No external UI libraries without discussion — keep the bundle minimal.

## Data Flow
1. Fetch from `/api/v1/` → operator API server → controller-runtime cache.
2. CVE data: parse CSV from scan results endpoint.
3. Severity badge colors: critical=red-600, high=orange-500, medium=yellow-400, low=blue-400.

## Build Integration
After UI changes, run `make build-ui` to update the embedded static files before testing the full binary.
