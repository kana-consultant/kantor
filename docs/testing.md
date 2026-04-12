# Testing Guide

This repository now includes an automated verification layer for the most regression-prone backend logic plus a frontend production build smoke check in GitHub Actions.

## What Is Covered

### Backend unit tests

Current automated backend coverage focuses on:

- IP rate limiting middleware
- Tenant base URL resolution for multi-tenant links
- WhatsApp helper logic:
  - template rendering
  - phone normalization
  - cron parsing
- Date-only parsing shared across HRIS and marketing forms
- Mail delivery settings normalization and feature gating
- Subscription monthly/yearly cost helpers
- Reimbursement attachment filtering and cleanup helpers
- HRIS overview payroll aggregation from encrypted salary values

Run all backend tests:

```bash
cd backend
go test ./...
```

## Frontend verification

The frontend is currently verified through a production build smoke check:

```bash
cd frontend
npm ci
npm run build
```

This catches broken imports, route generation issues, and TypeScript/Vite build regressions even before a broader UI test layer is added.

## CI workflow

GitHub Actions runs:

- backend unit tests with `go test ./...`
- frontend build smoke check with `npm run build`

Workflow file:

- `.github/workflows/verify.yml`

## Next testing targets

The current suite is intentionally focused on core business logic. Good next additions:

- repository tests with PostgreSQL fixtures or testcontainers
- API handler tests for auth, reimbursement, and tracker flows
- browser-level regression tests for critical authenticated pages
