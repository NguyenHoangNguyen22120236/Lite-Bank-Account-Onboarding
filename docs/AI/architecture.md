# Onboarding architecture

## Scope and current implementation

PR 08 defines the target architecture and changes documentation only. Protobuf
files remain authoritative. Future contracts, migrations, and implementation need
their own staged approval and TDD review.

Currently StartApplication persists pending terms. AcceptTerms commits acceptance.
SubmitIdentity commits identity and PENDING_WORKFLOW_START, starts Temporal, then
conditionally records pending identity verification. The worker executes only
Await(false). Signals, Updates, business Activities, delivery recovery, and expiry
are absent. ReviewApplication, GetApplication, and UpdateApplicationStatus return
Unimplemented. Account is a placeholder. VerifyIdentity works against immutable
synthetic fixtures; ScreenSanctions is unimplemented.

## Target ownership

| Component | Responsibility |
|---|---|
| Main BE | Application data, business validation, domain transitions, transactions, review audit, customer APIs, durable delivery intent |
| Temporal workflow | Deterministic sequencing, durable waits/timers, Signal consumption, review arbitration, Activity scheduling |
| Activities | External I/O through service APIs; no direct application-table access |
| Third-party Service | Identity and sanctions results; business negatives differ from technical failure |
| Account Service | Account creation, account-number generation, account idempotency by application |
| PostgreSQL | Shared physical database with service-owned tables; no cross-service table access |
| Main BE delivery recovery | Deliver committed startup and Signal intents independently of customer retries |
| Notification provider | Deliver notifications without changing account or application outcomes |

```mermaid
flowchart LR
    Customer --> Main[Main BE]
    Banker --> Main
    Main -->|Application aggregate and delivery intent| DB[(Local PostgreSQL)]
    Recovery[Main BE delivery recovery] -->|Claim and acknowledge intents| DB
    Main -->|Post-commit Start and Signals; review Updates| T[Local Temporal Service]
    Recovery -->|Retry Start and Signals| T
    Workers[Workflow and Activity workers] -->|Poll tasks| T
    Workers -->|Activities over internal gRPC| Main
    Workers -->|Activities over internal gRPC| Third[Third-party Service]
    Workers -->|Activities over internal gRPC| Account[Account Service]
    Account -->|Own tables only| DB
    Workers -->|Notification Activity| Notify[Notification provider]
```

Development remains local-first with Compose PostgreSQL and Temporal Dev Server.
The later deployment target is Main BE/Third-party/Account on Cloud Run, workers
on a Cloud Run Worker Pool, Cloud SQL, Temporal Cloud, and SendGrid. This does not
authorize cloud changes. Internal APIs retain gRPC over HTTP/2 through ConnectRPC
handlers; local VerifyIdentity uses h2c.

## Clean Architecture and transactions

Handlers parse requests and map responses/errors through small consumer-owned
service interfaces. Use-case services coordinate ports. Domain entities own rules,
invariants, and transitions without I/O; pass time explicitly. Services do not
depend on PostgreSQL drivers or Temporal SDK clients.

StartApplicationService owns creation/start intent; AcceptTermsService and
SubmitIdentityService own customer actions/Signal intents. ReviewApplicationService
authenticates/authorizes and submits the Update; a review-recording use case
persists the accepted decision. Status and expiration use cases validate domain
decisions before writing. Each consumer defines only its required repository and
external methods. Keep the Temporal adapter in internal/application/workflow and
worker orchestration in internal/workflow.

ApplicationRepository owns Application aggregate persistence, including identity
and terms, even across multiple implementation files. Repositories own SQL,
mapping, locking, constraints, and sanitized error classification, not business
decisions. Delivery storage is infrastructure, not a business repository per table.
Main BE owns review audit; its precise aggregate boundary remains to be designed.

Related aggregate changes and delivery intent commit atomically through the
service-owned Transactor.WithinTx callback and the same transaction context.
Parent-row locking serializes customer actions, expiration, reviews, and status
writes. No Temporal or provider call occurs while holding that transaction.
The infrastructure Transactor implements Begin/Commit and bounded cancellation-safe
rollback, not orchestration or callback retries.

## Startup, Signals, and delivery recovery

The selected target uses a transactional outbox. StartApplication commits an
application in PENDING_TERMS_ACCEPTANCE plus start intent, then attempts startup.
Workflow ID always equals application_id. Startup never advances business status.
Delivery state is separate: retain PENDING_WORKFLOW_START for contract compatibility
and legacy data, but do not use it for newly created target applications.

AcceptTerms commits acceptance, PENDING_IDENTITY_SUBMISSION, and TermsAccepted
intent; SubmitIdentity commits identity, PENDING_IDENTITY_VERIFICATION, and
IdentitySubmitted intent. Both send Signals after commit. Successful APIs mean
durable business data and delivery intent, not completed orchestration. Cancellation
after commit cannot undo persistence or cancel independent recovery.

A Main BE-owned recovery use case retries pending intents even without customer
retries. Establish startup before dependent Signals. Immediate delivery and recovery
share claim/acknowledgment rules. Crashes and lost responses can cause duplicates:
use stable event IDs, immutable committed references, and workflow guards.
Acknowledgment proves Temporal acceptance, not workflow processing. No exactly-once
delivery or cross-system transaction is assumed. Signals do not implicitly create
workflows. Missing executions trigger startup reconciliation; closed executions
require investigation rather than automatic recreation.

Signal payloads carry application ID, event ID, event type/version, and committed
record reference. Activities fetch authoritative data through Main BE APIs, which
do not yet exist. Avoid full identity in customer-action messages. Identity passed
to downstream Activities can still enter workflow history; access, retention, and
payload protection require review before implementation.

## Durable orchestration and human review

Register Signal consumers and the review Update handler early. Record received
facts deterministically; use workflow Await/selectors/timers, never busy loops or
process-local sleep. Buffer early/reordered actions and proceed only after both
committed actions are established. Duplicate Signals cannot repeat completed
business transitions.

Human review uses review_request_id as stable Update identity. Pure Update
validation performs no I/O. The handler checks phase and reserves one decision
before yielding to a persistence Activity. Main BE remains the business authority:
its service transaction locks, applies domain rules, and persists review audit,
request result, and APPROVED/REJECTED. Only then does the Update complete
successfully. Concurrent competing decisions cannot both win. Exact retries
return the durable result; the same ID with a changed payload conflicts.
Temporal deduplication does not replace durable review-request storage.
Do not close the workflow before required handlers and persistence finish.

Both sanctions clearance and human approval pass through APPROVED, then
PENDING_ACCOUNT_CREATION. Activities create accounts idempotently by application_id.
Main BE records COMPLETED only after an account is established. Notification
delivery is tracked separately and never changes the terminal business outcome.

## Terminal outcomes and abandoned applications

COMPLETED, REJECTED, FAILED_TECHNICAL, and proposed EXPIRED are terminal.
FAILED_TECHNICAL is not resumed by normal retries. Retry before terminal
classification; reconcile uncertain external effects first. Disable automatic
workflow execution retries that would restart onboarding.

For training simplicity, expire only customer inactivity: seven days from creation
to accept terms, then seven days from persisted acceptance to submit identity.
These are selected target defaults, configurable before implementation. Human
review waits durably without automatic expiry; an overdue alert threshold remains
to be selected. No new cancellation endpoint is proposed.

Use durable timers with deadlines based on persisted timestamps. Delayed startup
or Signals never reset deadlines. An expiration Activity asks Main BE to lock and
expire only if the action is still absent and the deadline has passed. A timely
committed action wins over late Signal delivery; return that fact so orchestration
can continue. Main BE rejects new actions at or after the deadline even if a timer
has not run. Exact replay of an already committed action remains valid.

EXPIRED needs a future additive protobuf enum and PostgreSQL enum migration.
Do not disguise abandonment as rejection or technical failure. In the target,
completed_at records every terminal outcome, including expiry and technical
failure; this supersedes the previous recoverable FAILED_TECHNICAL description.

## Remaining implementation decisions

See db.md for the mismatch inventory. Still decide retry budgets, notification
tracking, internal read/result contracts, authenticated reviewer propagation,
review aggregate boundary, customer identity mapping, and user/customer ownership.
Existing placeholder executions need replay-compatible versioning and explicit
migration/reconciliation; deployment alone does not turn Await(false) into this
lifecycle. Retain duplicate-start policies for retained executions and use durable
application state to prevent recreation after Temporal history retention expires.