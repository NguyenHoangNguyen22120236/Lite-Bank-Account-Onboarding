# Downstream Call Retry Policy

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