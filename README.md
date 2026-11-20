# Lite Bank Account Onboarding

A Go backend training project for a lightweight bank account onboarding workflow. The API design covers starting an application, accepting terms, submitting identity details, verifying identity, screening sanctions, reviewing applications, and checking progress.

The project uses Protocol Buffers and Connect RPC. Version 0.2.1 adds architecture, workflow, retry, and database reference documentation to the existing API contracts, generated clients and server interfaces, two runnable HTTP service entry points, and account service and Temporal worker scaffolding. Business handlers, services, repositories, third-party mocks, workflows, activities, and clients remain placeholders; both runnable servers register generated unimplemented handlers.

## Current Structure

```text
.
|-- api/
|   |-- api-design/api-docs.md       # API design and request/response examples
|   `-- proto/onboarding/v1/        # Application and third-party RPC contracts
|-- cmd/
|   |-- account/main.go            # Account command placeholder (no main function)
|   |-- worker/main.go             # Worker command placeholder (no main function)
|   |-- application/main.go        # Application server on :8080
|   `-- thirdparty/main.go         # Third-party server on :8081
|-- gen/onboarding/v1/              # Generated protobuf types and Connect bindings
|-- docs/
|   |-- AI/                        # Mermaid diagrams, retry policy, and table descriptions
|   `-- Human/                     # Links to original diagram sources
|-- internal/
|   |-- account/                  # Domain, handler, service, and repository placeholders
|   |-- workflow/                 # Workflow, review update, activity, and client placeholders
|   |-- application/
|   |   |-- domain/                # Domain placeholders
|   |   |-- handler/               # RPC handler placeholders
|   |   |-- repository/            # Repository interface and PostgreSQL placeholders
|   |   `-- service/               # Business logic placeholders
|   `-- thirdparty/
|       |-- handler/               # Third-party handler placeholders
|       |-- mock/                  # Identity and sanctions mock placeholders
|       `-- service/               # Third-party service placeholders
|-- buf.yaml                       # Protobuf module and lint configuration
|-- buf.gen.yaml                   # Protobuf and Connect code generation
|-- go.mod
|-- go.sum
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

The diagrams describe intended behavior, including later cloud deployment, rather than the current local scaffold. Database descriptions are discussion references, not migrations. Uncertain schema details and inconsistent relationship descriptions still require confirmation. Protobuf files remain the authoritative API contracts.

## How to Run Locally

### Prerequisites

- Go 1.26.3 or later, as declared in `go.mod`.
- Available local ports `8080` and `8081`.
- Network access to download Go dependencies on the first run.

Run all commands from the repository root. No database or environment variables are required for the current scaffold.

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

## Development Tooling and Limitations

The Makefile provides `proto`, `lint`, `fmt`, `vet`, `test`, `test-race`, `build`, and `ci` targets. With Make installed, `make run-app` and `make run-thirdparty` start the existing HTTP servers.

The account and worker commands contain only package declarations and have no `main` functions. Consequently, `make run-account`, `make run-worker`, the combined `make run`, and full executable builds cannot succeed yet. The Docker Compose targets require a Compose configuration, which is not included. Temporal and PostgreSQL dependencies and database migrations are also not included.

GitHub Actions checks protobuf lint and generation consistency, Go formatting, static analysis, race-enabled tests, and builds on pull requests and pushes to `main`. Test files currently contain package declarations only and do not verify behavior.

[AGENTS.md](AGENTS.md) describes engineering and human review requirements and points to the design documents under `docs/AI/`.
