# Retry, delivery, and failure policy

PR 08 target only. Current SubmitIdentity startup uses a request-derived three-second
budget, no application retry loop, and post-commit conditional status reconciliation.
No outbox/recovery exists today. The target moves startup to StartApplication and
replaces client-only recovery with durable delivery intents.

## Outcome rules

Business negatives are not technical errors: identity mismatch rejects; sanctions
match requires human review; banker rejection rejects. Transient failures retry
before any terminal classification. FAILED_TECHNICAL is terminal, not resumable.
Never automatically restart a terminal application or configure workflow execution
retries that repeat the lifecycle. A new customer attempt needs a new request_id.

Use bounded Activity retries with backoff and explicit timeouts. Exact intervals,
attempt limits, total processing budgets, delivery alert thresholds, and review
overdue thresholds remain decisions for implementation. No numeric SDK defaults
are asserted here. A delivery attempt has a bounded deadline; durable recovery
continues independently of the original HTTP request.

## Operation-specific handling

| Operation/condition | Target behavior |
|---|---|
| Database transaction fails | No intentional Temporal delivery; rollback related aggregate data and intent |
| Commit result is uncertain | Resolve by stable request/action identity; do not assume rollback or send uncommitted data |
| Crash after commit, before delivery | Recovery claims the durable intent without requiring customer replay |
| Start accepted, response lost | Retry same application_id; reconcile existing execution |
| Startup pending while customer acts | Commit actions/intents normally; establish start before dependent Signals |
| Signal accepted, acknowledgment persistence fails | Redeliver same event identity; workflow handles duplicates |
| Duplicate/reordered Signal | Retain facts; guard phase and prerequisites; no repeated transition |
| Signal target missing | Reconcile start intent and durable application state; no blind SignalWithStart |
| Workflow already closed | Compare persisted outcome; acknowledge stale intents only after reconciliation; never recreate automatically |
| Worker offline/restarted | Server retains accepted history; durable waits resume when compatible worker returns |
| Concurrent recovery workers | Short claim transaction and bounded lease; no network I/O under lock; expired claims are reclaimable |
| Review Update timeout after acceptance | Retry same review_request_id/payload; return durable result |
| Review persistence Activity retry | Same audit/request identity; no overwrite of accepted competing review |
| Status Activity response lost | Same transition replay returns established result; newer state is preserved |
| Account response lost | Retry/read using application_id; return existing account |
| Account exists but completion write fails | Retry completion persistence; never create another account or declare failure solely from uncertainty |
| Notification response lost | Retry stable notification identity; duplicates remain possible if provider cannot deduplicate |
| Notification retries exhausted | Record delivery failure and alert; preserve COMPLETED/REJECTED |
| Expiration races with action | Main BE serializes under parent lock and checks persisted action/deadline |
| Workflow fails, times out, cancels, or terminates abnormally | Recovery reconciles persisted state/external effects; persist terminal technical failure only when safe |

Delivery retries are at least once. Outbox acceptance is not evidence of workflow
processing or business completion. Separate delivery attempts from lifecycle
failure: a temporary Temporal outage does not immediately fail the application.
Persistent startup/delivery faults require alerting and reconciliation; after
classified unrecoverable failure, Main BE may atomically terminalize the application
and retire pending intents. The exact operational cutoff remains to be selected.

## Downstream classification

| Result | Retry/action |
|---|---|
| gRPC Unavailable, temporary network error | Bounded backoff |
| DeadlineExceeded | Retry only after considering whether the operation committed |
| ResourceExhausted / provider HTTP 429 | Backoff; honor provider retry hints |
| Transient provider HTTP 5xx | Bounded backoff |
| InvalidArgument | No blind retry; customer validation leaves the waiting stage unchanged |
| Unauthenticated / PermissionDenied; HTTP 401/403 | No blind retry; repair configuration and alert |
| NotFound | Reconcile ownership/existence; retry only if contract permits eventual consistency |
| AlreadyExists / FailedPrecondition; HTTP 409 | Distinguish exact replay from business conflict; do not suppress real conflicts |
| verified=false | REJECTED at IDENTITY_VERIFICATION; no technical retry |
| clear=false | PENDING_HUMAN_REVIEW; durable wait |
| Banker rejection | REJECTED at HUMAN_REVIEW; no technical retry |

VerifyIdentity itself still makes one provider call with caller context and no
retry loop. It does not persist results or reject applications. Its immutable
fixture provider has no network I/O. Future caller Activities own retry behavior.
Sanitize errors and never place customer data or credentials in logs/alerts.

## Terminal persistence and recovery

Main BE enforces valid conditional transitions through domain rules and
Transactor.WithinTx. An exact transition replay succeeds; a delayed request must
not overwrite a newer state. expected_status alone is not a complete transition
or idempotency policy. Define outcome identity for each future internal operation.

COMPLETED, REJECTED, FAILED_TECHNICAL, and proposed EXPIRED are terminal.
Normal workflow closure follows required persistence and completion of active
Update handlers. If Main BE is unavailable, preserve the recoverable workflow task/
Activity state or alert recovery rather than claiming terminal persistence succeeded.
Reconciliation of abnormal closure must survive process restarts and cannot rely
on a request goroutine. Existing terminal records prevent recreation after
Temporal execution history retention expires.

No automatic FAILED_TECHNICAL-to-processing transition, reset, or closed-workflow
restart is part of this design. Any administrative correction needs a separate
reviewed process. Reconcile a possibly-created account before failing onboarding.

## Abandoned customer actions

Use durable workflow timers: terms deadline = creation + seven days; identity
deadline = persisted terms acceptance + seven days. These training defaults are
configurable before implementation. Main BE independently enforces deadlines.
At or after the deadline a new action fails; exact replay remains successful.
On timer firing, an Activity locks and checks authoritative data. A timely
committed action proceeds even if its Signal was late; otherwise persist EXPIRED.
No automatic expiry is applied to human review or account creation. No expiry
notification is required in this training scope.

## Future verification

Use table-driven domain tests for transitions, time boundaries, and terminal guards.
Service unit tests use Mockery-generated consumer ports and Testify to verify
commit-before-delivery, no delivery on failed commit, replay, cancellation, and
error propagation. PostgreSQL integration tests cover atomic intent persistence,
rollback, leases/claim concurrency, constraints, and action/expiry races.
Temporal SDK tests cover early/duplicate/reordered Signals, durable timers,
concurrent Updates, handler completion, and retry-safe Activities. Live Temporal
integration tests cover actual message behavior, uncertain delivery, and worker
restarts in an approved disposable environment. Retain existing repository unit
tests until replacement/removal is explicitly approved.