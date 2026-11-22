# Lite Bank Account Onboarding

A Go backend training project for a lightweight bank account onboarding workflow. The API design covers starting an application, accepting terms, submitting identity details, verifying identity, screening sanctions, reviewing applications, and checking progress.

The project uses Protocol Buffers and Connect RPC. Version 0.5.0 adds the Application domain model and PostgreSQL repository for creation and lookup by application ID or request ID, with duplicate-key classification, unit tests, and opt-in PostgreSQL integration tests. It builds on the Goose migrations and reusable pgxpool connection helper introduced in 0.4.0. Local PostgreSQL and Temporal Dev Server infrastructure is available through Docker Compose, with persistent storage, health checks, and environment configuration. Business handlers, services, account repositories, third-party mocks, workflows, activities, and clients remain placeholders; both runnable servers register generated unimplemented handlers. Services do not yet use the database helper or application repository.

## Current Structure

```text
.
|-- api/
|   |-- api-design/api-docs.md       # API design and request/response examples
|   `-- proto/onboarding/v1/        # Application and third-party RPC contracts
|-- cmd/
|   |-- account/main.go            # Buildable Account placeholder; exits immediately
|   |-- worker/main.go             # Buildable Worker placeholder; exits immediately
|   |-- application/main.go        # Application server on :8080
|   `-- thirdparty/main.go         # Third-party server on :8081
|-- gen/onboarding/v1/              # Generated protobuf types and Connect bindings
|-- docs/
|   |-- AI/                        # Mermaid diagrams, retry policy, and table descriptions
|   `-- Human/                     # Links to original diagram sources
|-- internal/
|   |-- platform/postgres/         # Reusable pgxpool connection lifecycle helper
|   |-- account/                  # Domain, handler, service, and repository placeholders
|   |-- workflow/                 # Workflow, review update, activity, and client placeholders
|   |-- application/
|   |   |-- domain/                # Application model, statuses, and constructor
|   |   |-- handler/               # RPC handler placeholders
|   |   |-- repository/            # Application repository interface and PostgreSQL implementation
|   |   `-- service/               # Business logic placeholders
|   `-- thirdparty/
|       |-- handler/               # Third-party handler placeholders
|       |-- mock/                  # Identity and sanctions mock placeholders
|       `-- service/               # Third-party service placeholders
|-- buf.yaml                       # Protobuf module and lint configuration
|-- buf.gen.yaml                   # Protobuf and Connect code generation
|-- go.mod
|-- go.sum
|-- migrations/                    # Goose SQL migrations for application-owned tables
|-- compose.yaml                    # Local PostgreSQL and Temporal Dev Server
|-- .env.example                    # Development database settings
|-- .github/workflows/ci.yaml       # Protobuf and Go verification pipeline
|-- AGENTS.md                       # Engineering and review instructions
|-- Makefile                        # Generation, verification, and local run targets
|-- CHANGELOG
`-- VERSION
```

## API Endpoints

See [API documentation](api/api-design/api-docs.md) for the intended behavior and request/response examples. The [protobuf contracts](api/proto/onboarding/v1/) define the current message fields and enum names; some names in the design document differ from these contracts.

## Design Documentation

- [Architecture and component ownership](docs/AI/architecture.md)
- [Business onboarding flow](docs/AI/onboarding-flow.md)
- [Happy-case and human-review sequences](docs/AI/onboarding-sequence.md)
- [Downstream retry policy](docs/AI/retry-policy.md)
- [Database tables and relationships](docs/AI/db.md)
- Original sources: [system diagrams](docs/Human/diagrams.md) and [database diagram](docs/Human/db.md)

The diagrams describe intended behavior, including later cloud deployment, rather than the current local scaffold. The database reference records eight tables in one shared PostgreSQL database. PR 01 supplies migrations for three application-owned tables only: applications, terms acceptances, and application identities. Account/customer/user/review tables and their business behavior remain deferred. Customer identity mapping, review-request persistence, and other remaining design decisions are listed in [the database reference](docs/AI/db.md#remaining-decisions-and-contract-differences). Protobuf files remain the authoritative API contracts.

## How to Run Locally

### Prerequisites

- Go 1.26.3 or later, as declared in `go.mod`.
- Docker Desktop (Linux containers) or Docker Engine with Docker Compose v2.20+.
- GNU Make for the Makefile shortcuts; equivalent Docker commands are shown below.
- Available local ports `5432`, `7233`, `8233`, `8080`, and `8081`.
- Network access to download container images and Go dependencies on the first run.

Run all commands from the repository root. Only PostgreSQL and Temporal run in Docker. Application, Account, Third-party, and Temporal Worker run on the host; Account and Worker are still placeholders.

### Start local infrastructure

Copy the development settings (use `Copy-Item .env.example .env` in PowerShell):

```sh
cp .env.example .env
```

`.env` is gitignored. Its example credentials are for local development only. Compose reads these settings automatically; it does not export them to locally running Go processes. PostgreSQL initialization settings apply only when its data volume is empty; changing `.env` does not change existing database credentials.

Validate and start the infrastructure:

```sh
docker compose config --quiet
make up
# Without Make: docker compose up -d --wait --wait-timeout 120
```

`make up` waits for both health checks. The first run downloads the pinned official images, `postgres:17.12` and `temporalio/temporal:1.10.1`. Temporal uses its embedded SQLite database in a separate volume; it does not depend on PostgreSQL. See the [PostgreSQL image documentation](https://hub.docker.com/_/postgres) and [Temporal CLI documentation](https://github.com/temporalio/cli#run-via-docker).

| Dependency | Address from local applications | Settings |
|---|---|---|
| PostgreSQL | `localhost:5432` | Database, user, and password from `.env` |
| Temporal gRPC | `localhost:7233` | Namespace `default`; no TLS |
| Temporal Web UI | `http://localhost:8233` | Browse workflow state |

Published ports bind to loopback for local development. Compose does not apply migrations; use the explicit commands below. Each service remains responsible for its own data when repositories are implemented.

### Database migrations and connections

Goose v3.24.3 is pinned as a Go tool in `go.mod`; the connection helper uses pgx v5.7.6. Start local PostgreSQL first, then set `DATABASE_URL` explicitly for the target database. Neither Go nor the migration targets load `.env`, and the existing `POSTGRES_*` settings do not automatically construct `DATABASE_URL`.

In Bash, using the example local settings:

```bash
export DATABASE_URL='postgres://onboarding:onboarding_local@localhost:5432/onboarding?sslmode=disable'
make migrate-status
make migrate-up
make migrate-status
```

Repeat the export when opening a new terminal. Adjust the credentials and database name to match your PostgreSQL setup. The final status should show the migration as applied.

Without Make, configure Goose explicitly in Bash:

```bash
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="$DATABASE_URL"
go tool goose -env none -dir migrations status
go tool goose -env none -dir migrations up
```

In PowerShell, the equivalent direct commands are:

```powershell
$env:DATABASE_URL = 'postgres://onboarding:onboarding_local@localhost:5432/onboarding?sslmode=disable'
$env:GOOSE_DRIVER = 'postgres'
$env:GOOSE_DBSTRING = $env:DATABASE_URL
go tool goose -env none -dir migrations status
go tool goose -env none -dir migrations up
```

With GNU Make, `make migrate-status` and `make migrate-up` map `DATABASE_URL` to Goose's environment settings automatically. The `-env none` flag disables Goose's default `.env` loading. The first tool invocation may download dependencies. Up is transactional and repeated Up calls leave applied migrations unchanged. No service applies migrations on startup.

For disposable development/test data only, `make migrate-down` (or `go tool goose -env none -dir migrations down` with the environment above) rolls back the last migration. **Down deletes the three application tables and their data.** Do not use it to roll back valuable data.

`internal/platform/postgres.Open(ctx, dsn)` returns a pool only after a successful ping, uses pgxpool defaults, and closes the pool on initialization failure. Callers should supply a startup timeout and close the returned pool at shutdown. Errors omit driver details that may expose credentials; context cancellation/deadline errors remain detectable with `errors.Is`. The application PostgreSQL repository accepts this pool and does not own its lifetime. Connection retries and service wiring remain deferred; no service command currently opens a database connection.

### Application repository tests

`make test` runs `go test ./...`; `make test-race` runs the same suite with the race detector. Unit tests require no database. PostgreSQL integration tests skip when `TEST_DATABASE_URL` is absent. When it is supplied, connection or schema errors fail the tests.

To enable integration tests, point the variable at a disposable test database with the existing migration already applied. For example, in Bash (replace the connection settings with your test database settings):

```bash
export TEST_DATABASE_URL='postgres://onboarding:onboarding_local@localhost:5432/onboarding_test?sslmode=disable'
make test
make test-race
```

To run only the PostgreSQL integration tests and bypass cached results:

```bash
go test -v -count=1 ./internal/application/repository/postgres -run '^TestIntegration'
```

In PowerShell, set `$env:TEST_DATABASE_URL` to the test connection URL before running the same test commands. Neither the tests nor Make load this variable from `.env`. Tests create unique application IDs and clean up only their own rows; they do not create the database, apply migrations, or truncate tables.

Integration coverage includes nullable completion/rejection fields and enum round-trips through both lookup methods, duplicate request IDs without overwriting existing data, and concurrent inserts for one request ID. `Create` reports `ErrDuplicateRequestID`; callers can retrieve the original application with `GetByRequestID`. Service-level request replay and Temporal startup are deferred.

### Verify infrastructure

```sh
docker compose ps
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1;"'
docker compose exec temporal temporal operator cluster health
```

Expect both containers to be healthy, PostgreSQL to return `1`, and Temporal to report `SERVING`. Open `http://localhost:8233` to check the Web UI. On PowerShell, the SQL check can instead be run with `docker compose exec postgres psql -U onboarding -d onboarding -c "SELECT 1;"` when using the example settings.

```sh
make logs
# Without Make: docker compose logs -f --tail=100
```

Press `Ctrl+C` to stop following logs. If startup fails, check Docker is running, the published ports are available, and the logs show no initialization errors. A local PostgreSQL installation may already occupy port `5432`; stop it before starting this Compose stack. Running `make up` again reuses existing volumes.

### Run the local Go services

The current HTTP scaffold does not yet connect to PostgreSQL or Temporal.

Install dependencies:

```sh
go mod download
```

Start the application service in one terminal:

```sh
go run ./cmd/application
```

Start the third-party service in another terminal:

```sh
go run ./cmd/thirdparty
```

Check both services from Bash or a macOS terminal:

```sh
curl -fsS http://localhost:8080/health; printf '\n'
curl -fsS http://localhost:8081/health; printf '\n'
```

Expected responses are `application service healthy` and `third-party service healthy`. These checks confirm that the HTTP servers are running; the onboarding workflow is not yet implemented. Stop each server with `Ctrl+C` in its terminal.

Generated code is already included. If you change protobuf contracts later, install the Buf CLI and regenerate bindings with `buf generate` from the repository root. Generation uses the remote plugins configured in `buf.gen.yaml` and requires network access.

### Shut down infrastructure

```sh
make down
# Without Make: docker compose down
```

Shutdown removes the containers and network while preserving the `postgres_data` and `temporal_data` named volumes. A subsequent `make up` restores database and workflow state. Avoid `docker compose down --volumes` unless you intentionally want to delete all local infrastructure data. Temporal Dev Server is intended for development, not production deployment.

## Development Tooling and Limitations

The Makefile provides `proto`, `lint`, `fmt`, `vet`, `test`, `test-race`, `build`, and `ci` targets. With Make installed, `make run-app` and `make run-thirdparty` start the existing HTTP servers.

All four commands have `main` functions and are buildable. Account and worker have empty entry points and exit immediately; they do not serve requests or poll Temporal. Application and third-party retain their unimplemented RPC handlers. Database and Temporal runtime wiring remain deferred.

GitHub Actions checks protobuf lint and generation consistency, Go formatting, static analysis, migration-file validation, race-enabled tests, and builds on pull requests and pushes to `main`. Pool unit tests check invalid DSNs and credential redaction without a database. Application unit tests cover domain validation and repository error classification. CI currently provisions no PostgreSQL instance and sets no `TEST_DATABASE_URL`, so the repository integration tests skip there. Migration execution remains a separate manual check against disposable PostgreSQL. Most existing service test files remain placeholders.

[AGENTS.md](AGENTS.md) describes engineering and human review requirements and points to the design documents under `docs/AI/`.
