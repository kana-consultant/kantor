# Contributing to KANTOR

Thanks for contributing.

## Before You Start

- Review the repository structure and contribution rules in this document first
- Keep changes scoped to a clear problem or feature
- Do not commit secrets, `.env`, or production credentials
- Prefer small pull requests over large mixed changes

## Local Setup

1. Copy environment variables:

```bash
cp .env.example .env
```

2. Start the stack:

```bash
docker compose up --build -d
```

3. App endpoints:

- Frontend: `http://localhost:3000`
- Backend API: `http://localhost:3000/api/v1`

## Development Commands

Backend:

```bash
cd backend
go build ./cmd/server
```

Frontend:

```bash
cd frontend
npm install
npm run build
```

Full stack:

```bash
docker compose up --build -d
```

## Project Rules

- Follow the existing clean architecture pattern: handler -> service -> repository
- New API endpoints must include auth and RBAC unless intentionally public
- New frontend pages and actions must respect permission checks
- Use descriptive errors and keep validation explicit
- Do not edit already-applied migrations; create a new migration instead

## Pull Request Guidelines

Before opening a PR:

- Make sure the change builds successfully
- Test the affected flows manually
- Include screenshots for UI changes
- Mention environment or migration changes clearly
- Keep PR descriptions focused on behavior, risk, and verification

## Commit Style

Conventional-style commits are preferred, for example:

- `feat: add audit log viewer`
- `fix: stabilize employee avatar upload`
- `refactor: simplify sidebar mobile drawer`

## Reporting Bugs

Use the bug report issue template and include:

- Steps to reproduce
- Expected result
- Actual result
- Screenshots or logs if relevant

## Proposing Features

Use the feature request issue template and explain:

- The problem you want to solve
- The proposed solution
- Scope, risks, and affected modules if known
