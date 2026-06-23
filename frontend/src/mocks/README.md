# Frontend Mocks

This directory owns frontend-first mock data and API proxy handlers.

Use it for:
- Mock response fixtures that represent planned backend contracts.
- Local API proxy handlers for `/api/v1/*` routes while backend code is excluded from `codegym-v2`.
- Story and page data that should stay consistent across the app.

Keep `src/shared/api` focused on the real client contract. Keep mocks here so they can be removed or replaced cleanly when backend services are reintroduced.

Dev mode uses this mock API by default. Set `VITE_USE_MOCK_API=false` to let Vite proxy `/api` requests to a running backend.
