# KANTOR

KANTOR is an internal company platform that combines operational workflow, HRIS, and marketing management in one monorepo.

It includes:

- Operational project management and kanban workflows
- HRIS employee, finance, reimbursement, and subscription tracking
- Marketing campaign, ads metrics, and leads management
- RBAC with module-scoped access
- Audit logs, export/reporting, WA broadcast, and Chrome activity tracking

## Tech Stack

- Backend: Go
- Frontend: Vite, React, TanStack Router, TanStack Query
- Database: PostgreSQL
- Deployment: Docker / Docker Compose

## Repository Structure

```text
backend/     Go API, migrations, services, repositories
frontend/    Vite app, routes, components, hooks, services
extension/   Chrome extension tracker
docs/        Supporting documentation
```

## Quick Start

1. Copy the example environment file:

```bash
cp .env.example .env
```

2. Adjust required environment values in `.env`.

3. Start the local stack:

```bash
docker compose up --build -d
```

4. Open the app:

```text
http://localhost:3000
```

## Development

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

## Contributing

Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request.

## Security

Please read [SECURITY.md](./SECURITY.md) for vulnerability reporting instructions.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md).

## License

This project is licensed under the [MIT License](./LICENSE).
