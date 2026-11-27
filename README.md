# Lite Bank Account Onboarding

A Go backend training project for a lightweight bank account onboarding workflow. The API design covers starting an application, accepting terms, submitting identity details, verifying identity, screening sanctions, reviewing applications, and checking progress.

Current version: **0.10.1** — Application module refactor: focused use-case services, aggregate-root persistence, and service-owned transaction boundaries. See [CHANGELOG](CHANGELOG) for release details.

The project uses Protocol Buffers and Connect RPC. StartApplication validates client request UUIDs, persists applications, and resolves retries durably using the unique request ID. AcceptTerms records acceptance and advances the application status atomically. SubmitIdentity validates customer identity, saves it, and advances status to `PENDING_WORKFLOW_START` in one transaction. Both operations use PostgreSQL row locks for concurrent requests and support durable replay. The application server manages its pgxpool lifecycle and loads local `.env` configuration. Remaining application RPCs are unimplemented. CI runs PostgreSQL integration tests against a disposable database using existing Goose migrations. Local PostgreSQL and Temporal Dev Server infrastructure is available through Docker Compose. PR 06 starts Temporal after identity persistence and conditionally records pending identity verification. The local worker runs a deterministic durable wait. Business activities, account operations, and human review remain deferred.

## Current Structure

```text
.
|-- api/
|   |-- api-design/api-docs.md       # API design and request/response examples
|   `-- proto/onboarding/v1/        # Application and third-party RPC contracts
|-- cmd/
|   |-- account/main.go            # Buildable Account placeholder; exits immediately
|   |-- worker/main.go             # Temporal worker polling bank-onboarding
|   |-- application/main.go        # Application server on :8080
|   `-- thirdparty/main.go         # Third-party server on :8081
|-- gen/onboarding/v1/              # Generated protobuf types and Connect bindings
|-- docs/
|   |-- AI/                        # Mermaid diagrams, retry policy, and table descriptions
|   `-- Human/                     # Links to original diagram sources
|-- internal/
|   |-- platform/postgres/         # Reusable pgxpool connection lifecycle helper
|   |-- platform/temporal/         # Shared Temporal configuration and client lifecycle
|   |-- account/                  # Domain, handler, service, and repository placeholders
|   |-- workflow/                 # Durable waiting workflow; business activities remain placeholders
|   |-- application/
|   |   |-- domain/                # Application aggregate, identity, statuses, and domain rules
|   |   |-- handler/               # One ConnectRPC Handler; methods/interfaces split by use case
|   |   |-- repository/            # Aggregate ApplicationRepository and PostgreSQL transactor
|   |   |-- service/               # Focused use-case services, narrow interfaces, and WithinTx
|   |   `-- workflow/              # Main BE Temporal startup adapter; distinct from worker workflow code
|   `-- thirdparty/
|       |-- domain/                # Transport-independent identity and structural validation
|       |-- handler/               # VerifyIdentity ConnectRPC handler
|       |-- mock/                  # Immutable identity mock; sanctions placeholder
|       `-- service/               # Identity verification with injected provider
|-- buf.yaml                       # Protobuf module and lint configuration
|-- buf.gen.yaml                   # Protobuf and Connect code generation
|-- go.mod
|-- go.sum
|-- migrations/                    # Goose SQL migrations for application-owned tables
|-- compose.yaml                    # Local PostgreSQL and Temporal Dev Server
|-- .env.example                    # Development database and Temporal settings
|-- .github/workflows/ci.yaml       # Protobuf and Go verification pipeline
|-- AGENTS.md                       # Engineering and review instructions
|-- Makefile                        # Generation, verification, and local run targets
|-- CHANGELOG
`-- VERSION
```

## Application Module Organization

StartApplicationService, AcceptTermsService, and SubmitIdentityService each own
one use case and define only the repository methods they consume. One ConnectRPC
Handler implements the application service; each implemented RPC and its narrow
service interface live in a separate use-case file.

ApplicationRepository persists the application aggregate, including its identity
and terms records. SQL remains organized in application_repository.go,
application_identity_repository.go, and application_terms_acceptance_repository.go.
All methods share the existing PostgreSQL pool.

AcceptTerms and SubmitIdentity coordinate domain decisions and writes through the
database-independent `Transactor.WithinTx(ctx, fn)` interface. The PostgreSQL
transactor binds one transaction to the callback context and manages begin,
commit, and rollback. Application row locks remain held through transaction
completion. Temporal startup occurs only after identity persistence commits;
workflow startup reconciliation uses a separate transaction.

## API Endpoints

See [API documentation](api/api-design/api-docs.md) for the intended behavior and request/response examples. The [protobuf contracts](api/proto/onboarding/v1/) define the current message fields and enum names; some names in the design document differ from these contracts.

Implemented application APIs:

- **StartApplication:** creates an application in `PENDING_TERMS_ACCEPTANCE`; retries with the same request UUID return the existing application and current status.
- **AcceptTerms:** validates a non-nil application UUID and a nonblank, NUL-free term ID. It preserves the exact term value, records acceptance, and advances to `PENDING_IDENTITY_SUBMISSION` in one transaction. Repeating the recorded term after progression returns `accepted=true` without changing data. Different terms conflict; invalid state transitions fail. This flow permits one distinct term per application, while the schema retains its one-to-many capability. Neither API starts Temporal.
- **SubmitIdentity (PR 05–06):** validates identity and atomically saves it with `PENDING_WORKFLOW_START`, then attempts Temporal startup using the persisted workflow ID. New/running startup conditionally advances to `PENDING_IDENTITY_VERIFICATION`. Exact retries at pending-start retry startup; other progressed states return unchanged. Changed identity conflicts. Start and duplicate description share a three-second budget without an application retry loop. Uncertain failures preserve identity and pending-start for safe client retries; closed execution outcomes are reconciled without regressing application status.

PR 07 implements **ThirdPartyService.VerifyIdentity** over gRPC/HTTP/2 on port `8081` using a
replaceable mock provider with seven synthetic records. For example, `000000000001`, `Mock Citizen`,
`1990-01-01`, `2040-01-01` verifies when all four fields match exactly; unknown IDs
or mismatches return `verified=false`. Phone and email do not affect matching.
Input receives structural validation; age/current expiry eligibility stays in
Main BE. Technical errors return sanitized RPC errors, without retries or database
writes. See API 4 in the [API documentation](api/api-design/api-docs.md) for the
complete rules and request example. ScreenSanctions and Temporal business
activities remain deferred.

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

Run all commands from the repository root. Only PostgreSQL and Temporal run in Docker. Application, Account, Third-party, and Temporal Worker run on the host; Account remains a placeholder; Worker polls Temporal.

### Start local infrastructure

Copy the development settings (use `Copy-Item .env.example .env` in PowerShell):

```sh
cp .env.example .env
```

`.env` is gitignored. Its example credentials are for local development only. Compose reads these settings automatically. The application server and PostgreSQL integration tests also load the root `.env`, preserving existing environment variables. A missing file is allowed; malformed or unreadable files fail with a safe error. PostgreSQL initialization settings apply only when its data volume is empty; changing `.env` does not change existing database credentials.

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

Goose v3.24.3 is pinned as a Go tool in `go.mod`; the connection helper uses pgx v5.7.6. Start local PostgreSQL first, then set `DATABASE_URL` explicitly for migration commands. Migration targets do not load `.env`, and the existing `POSTGRES_*` settings do not automatically construct `DATABASE_URL`.

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

`internal/platform/postgres.Open(ctx, dsn)` returns a pool only after a successful ping, uses pgxpool defaults, and closes the pool on initialization failure. Errors omit driver details that may expose credentials; context cancellation/deadline errors remain detectable with `errors.Is`. The application server opens this pool using `DATABASE_URL` with a 10-second startup timeout, wires the repository, service, and ConnectRPC handler, and closes the pool after HTTP shutdown. Interrupt/SIGTERM allows up to 10 seconds for graceful shutdown before connections are closed. Startup does not retry connections or run migrations.

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

Alternatively, put `TEST_DATABASE_URL` in the repository root `.env`; PostgreSQL integration tests load it automatically from their package directory. Exported variables (including CI settings) take priority. Use only a disposable, already migrated test database. Tests create unique application IDs and clean up only their own rows; they do not create the database, apply migrations, or truncate tables.

Integration coverage includes nullable completion/rejection fields and enum round-trips through both lookup methods, duplicate request IDs without overwriting existing data, and concurrent inserts for one request ID. ConnectRPC-to-PostgreSQL tests cover StartApplication creation/replay, AcceptTerms persistence/replay, and SubmitIdentity persistence/replay, optional email presence, conflicts, invalid states, concurrent identical/conflicting submissions, and error mappings. Domain tests cover terms decisions and identity validation, calendar boundaries, equality, and state transitions; service tests cover validation, orchestration order, replay decisions, transaction failures, and workflow startup after successful commit. Transactor tests cover lifecycle errors, panic rollback, cancellation-safe cleanup, required transaction contexts, and concurrent context isolation. Integration tests verify that identity and terms inserts roll back when application status updates fail. Tests clean up identity and acceptance rows before their parent applications. PR 06 workflow startup is implemented; business activities remain deferred.

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

PR 06 connects Main BE and the worker to the existing local Temporal server.
Both read `TEMPORAL_ADDRESS` (default `localhost:7233`), `TEMPORAL_NAMESPACE`
(`default`), and `TEMPORAL_TASK_QUEUE` (`bank-onboarding`) from the environment or
root `.env`. Temporal SDK v1.51.0 and API v1.64.3 are pinned. Client connection
setup has a ten-second deadline and fails process startup if unavailable.

After the existing infrastructure is healthy, run these in separate terminals:

```sh
go run ./cmd/application
go run ./cmd/worker
```

The API can accept workflow startup even while the worker is offline. Submit an
application and repeat its exact identity request: both resolve the persisted
application ID as workflow ID, and only one execution should appear in the UI at
`http://localhost:8233`. Stopping/restarting the worker preserves the durable wait.
These are manual smoke checks, not commands automatically run by tests.

New/running startup conditionally advances pending-start to pending identity
verification. An existing completed execution requires a fresh database read and
does not imply identity verification. Only persisted `COMPLETED` or `REJECTED`
reconciles successfully. Unsuccessful closure conditionally records
`FAILED_TECHNICAL`; uncertainty retains pending-start and identity. Client retries
use the same workflow ID, with no application retry loop. Start and describe share
one three-second request-derived deadline. SDK transport retries remain bounded by
that deadline; workflow execution retries are not configured.

`REJECT_DUPLICATE` applies within Temporal's retained execution history; it is not
a permanent deduplication ledger after retention expiry. Progressed applications
skip startup. Worker tasks currently wait indefinitely without business work,
activities, or review messages. New code deployed later does not automatically
resume these placeholders into a future business flow. Placeholder migration and
recovery scanners remain out of scope. No protobuf or schema change is required.

The application server connects to PostgreSQL for StartApplication, AcceptTerms, and SubmitIdentity. PR 06 workflow startup is implemented; business activities remain deferred.

Install dependencies:

```sh
go mod download
```

Start PostgreSQL and apply the existing migration first (see above), then put the following URLs in the root `.env` (replace the example settings as needed):

```dotenv
DATABASE_URL=postgres://onboarding:onboarding_local@localhost:5432/onboarding?sslmode=disable
TEST_DATABASE_URL=postgres://onboarding:onboarding_local@localhost:5432/onboarding_test?sslmode=disable
```

The test database must exist and have migrations applied separately. Run the application from the repository root; it loads `.env` from the current working directory, so no Bash `source` command is needed. Existing environment variables take priority:

```sh
go run ./cmd/application
```

Start the third-party service in another terminal:

```sh
go run ./cmd/thirdparty
```

For manual VerifyIdentity testing, create a **gRPC request** in Postman targeting
`localhost:8081` with TLS disabled. Import
`api/proto/onboarding/v1/third_party.proto` and set the import root to `api/proto`
so `onboarding/v1/application.proto` resolves. Select
`onboarding.v1.ThirdPartyService/VerifyIdentity` and use a request from
[API 4](api/api-design/api-docs.md#api-4--verifyidentity). Import the proto instead
of using server reflection, which is not configured. Postman's JSON-shaped
message editor is encoded as protobuf over gRPC. Use the original protobuf field
names in Postman, for example:

```json
{
  "identity": {
    "id_number": "000000000001",
    "full_name": "Mock Citizen",
    "date_of_birth": "1990-01-01",
    "id_expiry_date": "2040-01-01",
    "phone_number": "0900000000"
  }
}
```

The seventh mock record is `079123456789`, `Nguyen Van A`, `2000-05-15`,
`2032-05-15`; use these values exactly when testing it.
The local server accepts HTTP/2 only, without TLS (h2c), and the integration test
uses `connect.WithGRPC()` over HTTP/2.
Verify an exact record (`verified=true`), an unknown ID (`verified=false`), and
malformed input (`InvalidArgument`). The service performs no provider retries or
application-state writes. ScreenSanctions still returns `Unimplemented`.

Check both services from Bash or a macOS terminal:

```sh
curl -fsS http://localhost:8080/health; printf '\n'
curl --http2-prior-knowledge -fsS http://localhost:8081/health; printf '\n'
```

Expected responses are `application service healthy` and `third-party service healthy`. These checks confirm that the HTTP servers are running; the workflow currently waits durably without business activities. Stop each server with `Ctrl+C` in its terminal.

The third-party check requires a curl build with HTTP/2 support; ordinary HTTP/1
requests will fail. On PowerShell, use `curl.exe` to avoid the `curl` alias.

Generated code is already included. If you change protobuf contracts later, install the Buf CLI and regenerate bindings with `buf generate` from the repository root. Generation uses the remote plugins configured in `buf.gen.yaml` and requires network access.

### Shut down infrastructure

```sh
make down
# Without Make: docker compose down
```

Shutdown removes the containers and network while preserving the `postgres_data` and `temporal_data` named volumes. A subsequent `make up` restores database and workflow state. Avoid `docker compose down --volumes` unless you intentionally want to delete all local infrastructure data. Temporal Dev Server is intended for development, not production deployment.

## Development Tooling and Limitations

The Makefile provides `proto`, `lint`, `fmt`, `vet`, `test`, `test-race`, `test-coverage`, `coverage-html`, `build`, and `ci` targets. With Make installed, `make run-app` and `make run-thirdparty` start the existing HTTP servers.

Generate and view coverage for the package list configured in the Makefile:

```sh
make test-coverage
make coverage-html
```

`coverage-html` reads the existing `coverage.out`; it does not regenerate it.
Run `make test-coverage` again after source files are renamed or moved to avoid
stale paths in the report. The coverage profile is gitignored. The configured
package list excludes handlers and Temporal adapters/workflows, so this is not
a whole-repository coverage report.

All four commands have `main` functions and are buildable. Account has an empty entry point; Worker polls Temporal and registers the durable waiting workflow. Application implements StartApplication, AcceptTerms, and SubmitIdentity with PostgreSQL persistence; its remaining RPCs return Unimplemented. Third-party implements VerifyIdentity using an injected mock provider; ScreenSanctions returns Unimplemented. PR 06 workflow startup is implemented; business activities remain deferred.

GitHub Actions checks protobuf lint and generation consistency, Go formatting, static analysis, migration-file validation, race-enabled tests, and builds on pull requests and pushes to `main`. Each verification job starts a PostgreSQL `postgres:17.12` service container with a disposable `onboarding_test` database and a readiness health check. CI sets `TEST_DATABASE_URL` with fixed test-only credentials and applies the existing migrations through Goose. It also starts a disposable `temporalio/temporal:1.10.1` development server, waits for its cluster health check, and sets `TEST_TEMPORAL_ADDRESS=localhost:7233` and `TEST_TEMPORAL_NAMESPACE=default`. A failed Temporal startup fails the job; container logs are printed and the container is removed during cleanup. CI runs `go test -race -count=1 -v ./...` so unit, PostgreSQL integration, and live Temporal integration tests run without cached results and appear in the logs. No local `.env`, Docker Compose stack, or worker is required by this job. The live Temporal test verifies that duplicate startup requests reuse the same running workflow and run ID. Pool unit tests cover invalid DSNs and credential redaction; Application tests cover domain validation, repository error classification, round-trips, duplicate request IDs, and concurrent inserts. Startup tests also cover Temporal options, deadline propagation, duplicate classification, safe retries, conditional status updates. The placeholder durable-wait unit test has been removed; live Temporal startup integration coverage remains. Manual smoke checks remain available; local race verification is blocked by the installed C compiler lacking 64-bit support.

[AGENTS.md](AGENTS.md) describes engineering and human review requirements and points to the design documents under `docs/AI/`.
