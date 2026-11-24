# Downstream Call Retry Policy

## PR 06 workflow startup exception

SubmitIdentity commits identity and `PENDING_WORKFLOW_START` before contacting
Temporal. Startup and duplicate description share one request-derived three-second
budget (an earlier caller deadline wins). There is no application retry loop;
SDK transport retries remain within that deadline. Workflow execution retries are
not configured. This is separate from the downstream activity policy below.

New/running executions conditionally advance pending-start to pending identity
verification. Confirmed failed, canceled, terminated, or timed-out executions
conditionally advance pending-start to `FAILED_TECHNICAL` and return
`FailedPrecondition`. A completed execution requires a fresh application read:
`COMPLETED` or `REJECTED` returns unchanged; other states are inconsistent and
return `FailedPrecondition` without writes. Workflow completion does not establish
identity verification.

Unavailable/uncertain starts preserve identity and pending-start. Clients may
retry the exact submission with the same persisted workflow ID. Deadline and
cancellation map to their corresponding API codes, temporary unavailability to
`Unavailable`, and configuration or database errors to sanitized `Internal`.
Database failure after accepted startup also permits this safe client retry.
Conditional status writes preserve states advanced by another writer. No recovery
scanner or restart of a closed execution is implemented.

This document separates **business decisions** (reject/approve/review) from **technical failures** (timeouts, temporary unavailability, invalid credentials). It accompanies docs/AI/onboarding-sequence.md

## Error classification and next action

| Response / condition | Retry? | Proposed strategy | Next action when retry is exhausted or not permitted |
|---|---|---|---|
| HTTP 500 or other transient server-side 5xx | Yes | Exponential backoff | Mark technical processing failure; raise an operations alert; retry/resume after repair |
| HTTP 429 Too Many Requests | Yes | Exponential backoff, honor `Retry-After` if supplied | Alert/queue operational recovery; do not treat as business rejection |
| Network timeout / connection error | Yes | Exponential backoff | Investigate downstream availability; recover the workflow when safe |
| HTTP 401 Unauthorized | No automatic retry | Fail fast | Repair credentials/configuration; alert operators |
| HTTP 403 Forbidden | No automatic retry | Fail fast | Investigate permissions; alert operators |
| HTTP 404 Not Found | Generally no | Fail fast unless the endpoint contract explicitly defines eventual consistency | Fix request or missing resource; do not silently re-create customer/account |
| HTTP 409 Conflict | Generally no | Check idempotency / business conflict contract | Return existing result for an idempotent duplicate, or surface real conflict |
| Invalid request / validation failure (4xx) | No | Fail fast | Return validation error; customer corrects input |
| Identity verification returns **not verified** | No | Business decision | Reject application; notify customer |
| Sanctions screening returns **match / not clear** | No | Business decision | Record pending human review; wait for banker decision |
| Banker rejects | No | Business decision | Reject application; notify customer |
| Account already exists for `application_id` | Not a retriable error | Return the same existing account result | Continue safely without creating a duplicate |
| SendGrid temporary failure | Yes | Bounded retry with backoff | Record notification failure and alert; **never roll back the created account** |
