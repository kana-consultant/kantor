# Contributing to KANTOR

Thank you for your interest in contributing to KANTOR! This guide will help you get started.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Development Commands](#development-commands)
- [Project Architecture](#project-architecture)
- [Project Rules](#project-rules)
- [Commit Style](#commit-style)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Reporting Bugs](#reporting-bugs)
- [Proposing Features](#proposing-features)

---

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally
3. Create a branch for your changes
4. Make your changes
5. Test your changes manually
6. Submit a pull request

## Development Setup

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) Engine 24+ and Docker Compose v2
- [Go](https://go.dev/dl/) 1.25+ (for backend development without Docker)
- [Node.js](https://nodejs.org/) 22+ (for frontend development without Docker)
- [Git](https://git-scm.com/)

### Quick Start

```bash
# Clone your fork
git clone https://github.com/<your-username>/kantor.git
cd kantor

# Copy environment variables
cp .env.example .env

# Start the full stack
docker compose up --build -d
```

The app will be available at `http://localhost:3000`.

### Endpoints

| Service | URL |
|---------|-----|
| Frontend | `http://localhost:3000` |
| Backend API | `http://localhost:3000/api/v1` (proxied through nginx) |
| PostgreSQL | `localhost:5432` (user: `dev`, password: `dev`) |

## Development Commands

### Backend

```bash
cd backend
go build ./cmd/server       # Build
go vet ./...                # Lint
go test ./...               # Test
```

### Frontend

```bash
cd frontend
npm install                 # Install dependencies
npm run dev                 # Development server (port 5173)
npm run build               # Production build
npm run lint                # Lint
```

### Full Stack (Docker)

```bash
docker compose up --build -d          # Start all services
docker compose logs -f backend        # Follow backend logs
docker compose down                   # Stop all services
docker compose up --build -d backend  # Rebuild backend only
```

### Database Migrations

Migrations run automatically on backend startup. To create a new migration:

```bash
# Create a new migration pair
touch backend/migrations/YYYYMMDDHHMMSS_description.up.sql
touch backend/migrations/YYYYMMDDHHMMSS_description.down.sql
```

> **Important:** Never edit already-applied migrations. Always create a new migration file.

## Project Architecture

```
Handler → Service → Repository → PostgreSQL
```

- **Handler**: HTTP request parsing, validation, JSON responses
- **Service**: Business logic, orchestration, error handling
- **Repository**: Pure SQL queries via pgx (no ORM)

See [docs/architecture.md](docs/architecture.md) for a detailed overview.

## Project Rules

- Follow the existing clean architecture pattern (handler → service → repository)
- New API endpoints **must** include auth and RBAC middleware unless intentionally public
- New frontend pages **must** check permissions via the `useRBAC()` hook or `PermissionGate` component
- Use descriptive error messages and keep validation explicit
- Do not edit already-applied migrations — create a new migration instead
- Do not commit secrets, `.env`, or production credentials
- Prefer small, focused pull requests over large mixed changes

## Commit Style

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add audit log viewer
fix: stabilize employee avatar upload
refactor: simplify sidebar mobile drawer
docs: update deployment guide
chore: update Go dependencies
```

## Pull Request Guidelines

Before opening a PR:

- [ ] Your code builds successfully (`go build ./...` and `npm run build`)
- [ ] You have tested the affected flows manually
- [ ] Include screenshots for UI changes
- [ ] Mention environment or migration changes clearly
- [ ] Keep the PR description focused on behavior, risk, and verification steps

### PR Template

```markdown
## Summary
Brief description of what this PR does.

## Changes
- Bullet list of key changes

## Test Plan
- Steps to verify the changes work correctly

## Screenshots
(if applicable)
```

## Reporting Bugs

[Open a bug report](https://github.com/kana-consultant/kantor/issues/new) and include:

- Steps to reproduce
- Expected result
- Actual result
- Screenshots or logs if relevant
- Environment details (OS, browser, Docker version)

## Proposing Features

[Open a feature request](https://github.com/kana-consultant/kantor/issues/new) and explain:

- The problem you want to solve
- The proposed solution
- Scope, risks, and affected modules if known
- Mockups or wireframes (if applicable)

---

Thank you for contributing!
