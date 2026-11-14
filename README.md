# Lite Bank Account Onboarding

A Go backend training project for a lightweight bank account onboarding workflow. The API design covers starting an application, accepting terms, submitting identity details, verifying identity, screening sanctions, reviewing applications, and checking progress.

The project uses Protocol Buffers and Connect RPC. It currently provides API contracts, generated clients and server interfaces, and two HTTP service entry points. Business handlers, services, repositories, and third-party mocks are placeholders; both servers currently register generated unimplemented handlers.

## Current Structure

```text
.
|-- api/
|   |-- api-design/api-docs.md       # API design and request/response examples
|   `-- proto/onboarding/v1/        # Application and third-party RPC contracts
|-- cmd/
|   |-- application/main.go        # Application server on :8080
|   `-- thirdparty/main.go         # Third-party server on :8081
|-- gen/onboarding/v1/              # Generated protobuf types and Connect bindings
|-- internal/
|   |-- application/
|   |   |-- domain/                # Domain placeholders
|   |   |-- handler/               # RPC handler placeholders
|   |   |-- repository/            # Persistence placeholders
|   |   `-- service/               # Business logic placeholders
|   `-- thirdparty/
|       |-- handler/               # Third-party handler placeholders
|       |-- mock/                  # Identity and sanctions mock placeholders
|       `-- service/               # Third-party service placeholders
|-- buf.yaml                       # Protobuf module and lint configuration
|-- buf.gen.yaml                   # Protobuf and Connect code generation
|-- go.mod
|-- go.sum
|-- CHANGELOG
`-- VERSION
```

## API Endpoints

See [API documentation](api/api-design/api-docs.md) for the intended behavior and request/response examples. The [protobuf contracts](api/proto/onboarding/v1/) define the current message fields and enum names; some names in the design document differ from these contracts.

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
