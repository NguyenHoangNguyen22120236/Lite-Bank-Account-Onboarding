# Codex Engineering Instructions

## 1. Project Context

This is a backend engineering training project for a lite bank account onboarding system.

Technology:
- Go
- ConnectRPC / gRPC / Protocol Buffers / Buf
- PostgreSQL
- Temporal
- Docker and Docker Compose
- GitHub Actions for CI
- Mockery for mock generation
- Testify for testing

Development principles:
- Clean Architecture
- Domain-Driven Design (DDD)
- SOLID principles
- Idiomatic Go
- Test-Driven Development (TDD)

Development is local-first.
Cloud deployment is a later project phase.

## 2. Documentation Sources

Read relevant documents before proposing changes:

- docs/AI/architecture.md
- docs/AI/onboarding-flow.md
- docs/AI/onboarding-sequence.md
- docs/AI/retry-policy.md
- docs/AI/db.md
- api/api-design/api-docs.md

Protobuf files are authoritative API contracts.

If documents and code conflict, report the conflict.
Do not silently invent or change requirements.

## 3. Architecture Principles

### 3.1 General Principles

- Follow Clean Architecture and pragmatic Domain-Driven Design.
- Maintain clear separation of handler, service, domain, and repository responsibilities.
- Follow SOLID principles and idiomatic Go conventions.
- Use constructor-based dependency injection.
- Define small interfaces in the consuming package.
- Depend on abstractions rather than concrete infrastructure implementations.
- Avoid unnecessary abstractions and premature generalization.
- Do not put business logic in main.go.

### 3.2 Handler Layer

- Handle transport-level responsibilities, including request parsing, response mapping, and RPC error mapping.
- Define consumer-owned service interfaces containing only the methods required by the handler.
- Do not depend directly on concrete service implementations.
- Do not implement business rules, database operations, or transaction orchestration.
- Preserve the approved transport protocol for each API.

### 3.3 Service Layer

- Organize services by business use case, such as StartApplicationService, AcceptTermsService, and SubmitIdentityService.
- Each service should have a cohesive responsibility.
- Define consumer-owned repository and external dependency interfaces.
- Interfaces must contain only the methods required by the consuming service.
- Coordinate domain operations, repository operations, transactions, and external service interactions.
- Delegate business rules, invariants, and state transition validation to the domain layer.
- Do not depend directly on PostgreSQL drivers, Temporal SDK clients, or other concrete infrastructure implementations.
- Do not implement SQL or transport-specific logic.

### 3.4 Domain Layer

- Own business entities, value objects, business rules, invariants, and state transition validation.
- Keep domain logic independent of handlers, services, repositories, databases, and external infrastructure.
- Prefer pure, deterministic business logic where possible.
- Do not perform database operations, network calls, or other external I/O.
- Pass time and other external inputs explicitly when required by business rules.

### 3.5 Repository Layer

- Design repositories around aggregate roots and consistency boundaries rather than database tables.
- Application is the aggregate root for application identity and terms acceptance.
- Identity and TermsAcceptance persistence belongs to ApplicationRepository.
- Repository implementations may be split into multiple files while maintaining the same aggregate repository.
- Repository implementations may depend on domain entities but must not execute domain business rules or determine business outcomes.
- Own SQL execution, data mapping, persistence, database locking, and database error classification.
- Use parameterized SQL.
- Use database constraints and atomic operations to enforce persistence-level integrity.
- Map infrastructure errors to repository-level errors without exposing sensitive database details.
- Preserve database ownership per service.
- Do not directly access another service's tables.

### 3.6 Transaction Management

- The service layer owns transaction boundaries for business use cases.
- Use the Transactor interface and WithinTx method to coordinate transactions.
- Keep transaction implementation details in the infrastructure layer.
- Repository operations within a transaction must use the same transaction context.
- Ensure related aggregate changes commit or roll back atomically.
- Use appropriate row-level locking and conditional updates to prevent race conditions.
- Do not place business orchestration inside repository transaction implementations.
- Avoid external network calls inside database transactions.
- Explicitly address consistency and recovery when coordinating PostgreSQL with external systems.

### 3.7 API Contracts and Generated Code

- Protobuf files define API contracts.
- Do not modify generated Protobuf or Mockery code manually.
- Use Buf to regenerate code after approved .proto changes.
- Preserve backward compatibility unless a breaking change is explicitly approved.

## 4. Temporal Principles

- Workflow code must remain deterministic.
- External I/O belongs in Activities.
- Keep Temporal client implementations separate from business services through consumer-owned interfaces.
- Activities must be safe under retries.
- Distinguish business rejection from technical failure.
- Do not assume exactly-once external side effects.
- Preserve idempotency across workflow startup, external events, and activity execution.
- Human review decisions must handle concurrency.
- Durable workflows must support worker restarts.
- Use durable waiting mechanisms instead of busy loops.
- Explicitly handle consistency between PostgreSQL and Temporal.
- Do not assume a database transaction can atomically commit a Temporal operation.
- Follow the approved workflow design and retry policy.

## 5. AI-Assisted Development Rules

For each feature:

1. Read the relevant specifications.
2. Inspect the existing implementation and architectural conventions.
3. Explain the intended behavior.
4. Propose a small implementation plan.
5. Identify affected files and dependencies.
6. Propose test cases and appropriate testing levels before implementation.
7. STOP and request human approval.
8. Write the approved tests and required generated mocks only.
9. Verify that new behavior tests fail for the expected reason.
10. STOP for human test review.
11. Implement the minimum code needed to pass.
12. Refactor without changing approved behavior.
13. Run relevant verification.
14. Report changes, verification, and remaining risks.

Do not implement an entire feature in one step unless explicitly authorized.

Do not automatically start another feature.

For behavior-preserving refactoring, establish a passing test baseline and keep existing tests green.

## 6. Test-Driven Development

### 6.1 TDD Process

Use Red -> Green -> Refactor.

- Red: approved tests fail for the expected reason.
- Green: minimal implementation passes the tests.
- Refactor: improve code while keeping tests green.

Prefer table-driven tests for Go business rules.

DO NOT modify approved test expectations merely to make incorrect implementation code pass.

### 6.2 Testing Strategy

**Domain Layer — Unit Tests**

- Test business rules, invariants, validation, and state transitions.
- Test boundary conditions and invalid inputs.
- Keep tests independent of infrastructure.
- Do not mock pure domain logic.

**Service Layer — Unit Tests**

- Test each business use case independently.
- Mock repository, Transactor, Temporal, and other external dependency interfaces where applicable.
- Verify business orchestration, error propagation, transaction coordination, and idempotency.
- Do not require a running database or external service.
- Test observable behavior rather than unnecessary implementation details.

**Repository Layer — Integration Tests**

- Use real PostgreSQL for repository testing.
- Test SQL execution, persistence, data mapping, constraints, locking, transactions, and rollback behavior.
- Test concurrent database operations and persistence-level idempotency.
- Do not introduce new repository unit tests that mock SQL or database drivers.
- Preserve existing repository unit tests until their removal or replacement is explicitly approved.
- Use isolated, disposable test databases.

**Temporal Workflow and Activities**

- Use the Temporal Go SDK testing facilities for workflow orchestration tests.
- Mock external dependencies when unit testing Activities.
- Use Temporal integration tests for behavior that requires a real Temporal server.

### 6.3 Mockery and Testify

- Use Mockery to generate mocks for dependency interfaces.
- Maintain mock generation configuration in .mockery.yml.
- Use the existing make mocks command to regenerate mocks.
- Regenerate affected mocks when interfaces change.
- Keep generated mocks in their configured test packages.
- Do not manually modify generated mocks.
- Do not create handwritten mocks where Mockery-generated mocks are suitable.
- Use Testify require and assert for test assertions.
- Use Testify mock expectations through generated mocks.
- Verify meaningful dependency interactions and expected call counts.
- Avoid overly strict mock expectations that couple tests to irrelevant implementation details.

### 6.4 Required Test Scenarios

Cover the following scenarios where applicable:

- Successful operations
- Validation failures
- Business rule violations
- Valid and invalid state transitions
- Duplicate requests and idempotent replay
- Concurrent requests
- Database and infrastructure failures
- Transaction commit and rollback
- Context cancellation and timeouts
- Temporal retries and worker restarts

Do not duplicate identical test scenarios across layers without a meaningful reason.

## 7. Execution and Permissions

Default to proposing commands instead of running them.

Ask before:

- Installing dependencies
- Running Docker commands
- Applying database migrations
- Modifying the database schema
- Making network calls
- Running destructive commands
- Pushing commits or creating pull requests
- Making cloud infrastructure changes
- Making API changes

The following local verification commands may run without asking, provided integration tests use an approved disposable test environment:

- go test ./...
- go test -race ./...
- go vet ./...
- buf lint
- git diff
- git status

Use the existing Makefile and project configuration for formatting, mock generation, testing, coverage, and builds.

For other commands, propose them first and request approval.

Read-only inspection can proceed when authorized.

Never expose secrets or real customer data.

Do not report skipped integration tests as successfully executed tests.

## 8. Git and Pull Requests

- One PR should have one cohesive objective.
- Keep PRs small and independently reviewable.
- Avoid mixing unrelated refactors with features.
- Include relevant tests alongside each implementation change.
- Keep generated code synchronized with its source contracts and interfaces.
- Update relevant documentation when architecture or behavior changes.
- Do not push commits, merge pull requests, or perform other Git operations that modify the remote repository without explicit approval.
- Once implementation and tests are complete, provide a concise pull request description summarizing changes, verification, and remaining considerations.
- All CI checks must pass before merging.

## 9. Communication Style

This is a learning project.

Before writing substantial code, explain:

- Why the change is needed
- Which layer owns the behavior
- Relevant architectural decisions
- Expected failure scenarios
- Concurrency and idempotency considerations
- Testing approach and trade-offs

Do not hide important decisions from the developer.

Challenge architectural decisions constructively when they introduce unnecessary complexity, coupling, or inconsistency.

Prefer understanding over maximum coding speed.

## 10. Definition of Done

A feature is complete when:

- Behavior matches approved requirements.
- Architecture follows the established project conventions.
- Approved unit and integration tests pass.
- Relevant failure and concurrency scenarios are covered.
- Generated mocks and Protobuf code are synchronized.
- Code is formatted and buildable.
- Relevant documentation is updated.
- Verification results and remaining risks are reported.
- Human review is complete.