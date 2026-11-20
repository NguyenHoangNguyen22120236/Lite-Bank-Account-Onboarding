
# Codex Engineering Instructions

## 1. Project Context

This is a backend engineering training project for
a lite bank account onboarding system.

Technology:
- Go
- ConnectRPC / Protocol Buffers / Buf
- PostgreSQL
- Temporal
- Docker and Docker Compose
- GitHub Actions for CI

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

- Maintain clear separation of handler, service,
  domain, and repository responsibilities.
- Keep domain logic independent from transport.
- Do not put business logic in main.go.
- Use dependency injection and small interfaces.
- Use parameterized SQL and transactions where needed.
- Preserve database ownership per service.
- Do not directly access another service's tables.
- Do not modify generated code manually.
- Use Buf to regenerate code after .proto changes.

## 4. Temporal Principles

- Workflow code must remain deterministic.
- External I/O belongs in Activities.
- Activities must be safe under retries.
- Distinguish business rejection from technical failure.
- Do not assume exactly-once external side effects.
- Human review decisions must handle concurrency.
- Durable workflows must support worker restarts.

## 5. AI-Assisted Development Rules

For each feature:

1. Read the relevant specifications.
2. Explain the intended behavior.
3. Propose a small implementation plan.
4. Identify affected files and dependencies.
5. Propose test cases before implementation.
6. STOP and request human approval.
7. Write the approved tests only.
8. STOP for human test review.
9. Implement the minimum code needed to pass.
10. Refactor without changing behavior.
11. Report changes, verification and remaining risks.

Do not implement an entire feature in one step
unless explicitly authorized.

Do not automatically start another feature.

## 6. Test-Driven Development

Use Red -> Green -> Refactor.

- Red: approved tests fail for the expected reason.
- Green: minimal implementation passes the tests.
- Refactor: improve code while keeping tests green.

Prefer table-driven tests for Go business rules.

Test:
- Success cases
- Validation failures
- State transitions
- Duplicate requests
- Concurrent requests
- Database failures
- Temporal retries and worker crashes

DO NOT modify approved test expectations merely to
make incorrect implementation code pass.

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

The following verification commands may run without asking:
- go test ./...
- go test -race ./...
- go vet ./...
- buf lint
- git diff
- git status

For other commands, propose them first and request approval.

Read-only inspection can proceed when authorized.
Never expose secrets or real customer data.

## 8. Git and Pull Requests

- One PR should have one cohesive objective.
- Keep PRs small and independently reviewable.
- Avoid mixing unrelated refactors with features.
- Include relevant tests alongside each implementation change.
- Do not push commits, merge pull requests, or perform other Git operations that modify the remote repository without explicit approval.
- Once implementation and tests are complete, provide a concise pull request description summarizing the changes, tests performed, and any remaining considerations.
- All CI checks must pass before merging.

## 9. Communication Style

This is a learning project.

Before writing substantial code, explain:
- Why the change is needed
- Which layer owns the behavior
- Expected failure scenarios
- Concurrency and idempotency considerations

Do not hide important decisions from the developer.
Prefer understanding over maximum coding speed.

## 10. Definition of Done

A feature is complete when:
- Behavior matches approved requirements
- Approved tests pass
- Relevant error cases are covered
- Code is formatted and buildable
- Relevant documentation is updated
- Human review is complete
