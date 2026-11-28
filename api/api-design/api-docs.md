# API behavior and PR 08 target

Protobuf files in api/proto/onboarding/v1 are authoritative. The field sketches
below summarize their contracts. PR 08 changes documentation only. Current
StartApplication only persists; AcceptTerms has no Signal; SubmitIdentity starts
Temporal after identity/pending-start commit. The worker only waits. Review,
GetApplication, UpdateApplicationStatus, sanctions screening, and account processing
remain unimplemented.

Target customer-action transactions atomically commit business data and outbox
intent through service-owned Transactor.WithinTx, then attempt delivery. Success
means durable data/intent, not Temporal processing. Recovery continues independently
of the HTTP request. No external call occurs inside a database transaction. Exact
business replay does not rewrite data; recovery can redeliver the original intent.
The existing three-second startup budget belongs to the current SubmitIdentity
adapter, not an approved target delivery policy.

## **Service:** ApplicationService — Main BE

### API 1 — StartApplication

**Purpose:** Initialize a new application and begin tracking it.

#### Request

```protobuf
StartApplicationRequest {
  request_id: string // Client-generated non-nil UUID, reused for retries.
}
```

#### Response

```protobuf
StartApplicationResponse {
  application_id: string
  status: ApplicationStatus
}
```

A new request persists an application in `APPLICATION_STATUS_PENDING_TERMS_ACCEPTANCE`.
Reusing `request_id` returns the existing application ID and current status without
overwriting persisted data. Invalid request IDs return `InvalidArgument`; context
cancellation and deadlines return `Canceled` and `DeadlineExceeded`. Other failures
return sanitized `Internal`. Target: commit startup intent with the application,
then attempt Start with application_id as Workflow ID after commit. Startup does
not advance business status. Delivery recovery continues without customer retries.
Terminal applications never start another execution.

### API 2 — AcceptTerms

**Purpose:** Record the customer's agreement to the terms before identity submission.

#### Request

```protobuf
AcceptTermsRequest {
    application_id: string
    term_id: string
}
```

#### Response

```protobuf
AcceptTermsResponse {
accepted: bool
}
```

`application_id` must be a non-nil UUID. `term_id` must not be empty,
whitespace-only, or contain NUL; its original case and whitespace are preserved.
The first acceptance atomically records the term and advances
`PENDING_TERMS_ACCEPTANCE` to `PENDING_IDENTITY_SUBMISSION`.
This flow permits one distinct term per application. Repeating the same recorded
term at any later state returns `accepted=true` without changing persisted data.
A different recorded term returns `AlreadyExists`; an invalid transition or
an acceptance already present in the initial state returns `FailedPrecondition`.
Invalid input returns `InvalidArgument`, an absent application returns `NotFound`,
and database failures return sanitized `Internal` errors. Cancellation and
deadlines return `Canceled` and `DeadlineExceeded` when observed by the request;
errors after commit do not imply rollback. Target: acceptance, status, and stable
TermsAccepted intent commit together, then Main BE signals the existing workflow.
accepted=true means durable acceptance, not Signal processing. Delivery recovery
establishes missing startup; AcceptTerms does not start a workflow itself.

### API 3 — SubmitIdentity

**Purpose**: Validate and persist customer identity details for asynchronous account opening.

Target behavior replaces current PR 06 startup: SubmitIdentity does not start
Temporal. It commits identity, PENDING_IDENTITY_VERIFICATION, and stable
IdentitySubmitted intent, then signals the already-started workflow. Successful
responses report persisted status, not verification completion. Failed or uncertain
delivery preserves data and intent for background recovery.

`application_id` must be a non-nil UUID and `identity` must be present. ID number
must contain exactly 12 ASCII digits, full name must be nonblank, and phone must
be nonblank with at most 20 Unicode characters. String fields must contain no NUL.
Dates must be real calendar dates in strict `YYYY-MM-DD` form, with year at least 1.
The applicant must be at least 18 and ID expiry must be strictly later than today,
using UTC calendar dates. February 29 birthdays reach the next age on March 1 in
non-leap years. No additional phone or email format validation is imposed.

Identity insertion and the transition from `PENDING_IDENTITY_SUBMISSION` to
`PENDING_IDENTITY_VERIFICATION`, together with Signal intent, commit atomically
under an application-row lock in the target. Submitted
values are preserved exactly, including whitespace, case, and absent versus present
empty email. Replaying the exact stored identity leaves identity unchanged and
returns the persisted current application status without business writes, even if the ID has since
expired. Structural validation still applies; current age/expiry checks apply only
to first submissions. Changing any identity field, including expiry or email
presence, conflicts. This API does not edit an already submitted identity.

Invalid input returns `InvalidArgument`; missing application returns `NotFound`;
conflicting identity returns `AlreadyExists`. Invalid transitions, including an
identity already present at either pre-submission status, return
`FailedPrecondition` without repairing data. Database failures return sanitized
`Internal` errors; cancellation and deadlines return `Canceled` and
`DeadlineExceeded`. Successful responses contain the persisted application ID and
status.

#### Request

```protobuf
SubmitIdentityRequest {
  application_id: string
  identity: IdentityInfo
}
```

#### Response

```protobuf
SubmitIdentityResponse {
  application_id: string
  status: ApplicationStatus
}
```

#### Example request

```json
{
  "application_id": "58ad37d4-7256-4416-9948-2c0e5cbad837",
  "identity": {
    "id_number": "079123456789",
    "full_name": "Nguyen Van A",
    "date_of_birth": "2002-05-15",
    "id_expiry_date": "2032-05-15",
    "phone_number": "0901234567",
    "email": "example@email.com"
  }
}
```

#### Example response

```json
{
  "application_id": "58ad37d4-7256-4416-9948-2c0e5cbad837",
  "status": "APPLICATION_STATUS_PENDING_IDENTITY_VERIFICATION"
}
```

<br>

## **Service:** Internal third-party APIs

### API 4 — VerifyIdentity

**Purpose**: Check whether the customer's identity is valid against mock government data.

#### Request

```protobuf
VerifyIdentityRequest {
    identity: IdentityInfo
}
```

#### Response

```protobuf
VerifyIdentityResponse {
    verified: bool
}
```

PR 07 implements `/onboarding.v1.ThirdPartyService/VerifyIdentity` through a
ConnectRPC handler, service, and injected mock provider. It does not persist
results or change application state.

The intended internal protocol is **gRPC over HTTP/2**, served by the existing
generated ConnectRPC handler. Locally, port `8081` accepts unencrypted HTTP/2
(h2c); the current server enables only unencrypted HTTP/2, including `/health`. No separate REST endpoint or native
gRPC handler is introduced. Generated Go clients select gRPC with
`connect.WithGRPC()` and use an HTTP/2-capable transport.

`identity` must be present. ID number must contain exactly 12 ASCII digits;
name and phone must be nonblank, with phone at most 20 Unicode characters.
All strings must be valid UTF-8 and NUL-free. Dates must be real calendar dates
in strict `YYYY-MM-DD` form with year at least 1. Email is optional without
format validation. Values are preserved exactly. Age and current expiry
eligibility remain Main BE rules; this API uses no clock.

The mock contains seven synthetic government records:

| ID number | Full name | Date of birth | ID expiry date |
|---|---|---|---|
| `000000000001` | `Mock Citizen` | `1990-01-01` | `2040-01-01` |
| `000000000002` | `Mock Citizen Two` | `1985-06-15` | `2041-06-15` |
| `000000000003` | `Mock Citizen Three` | `1995-03-20` | `2042-03-20` |
| `000000000004` | `Mock Citizen Four` | `2000-09-10` | `2043-09-10` |
| `000000000005` | `Mock Citizen Five` | `1988-12-25` | `2044-12-25` |
| `000000000006` | `Mock Citizen Six` | `1992-02-29` | `2044-02-29` |
| `079123456789` | `Nguyen Van A` | `2000-05-15` | `2032-05-15` |

All four fields must match exactly. Unknown IDs or mismatches return RPC success
with `verified=false`, a business verification failure. No trimming or case
normalization occurs: ` Mock Citizen ` is structurally valid but does not match.
Phone and email do not affect matching. Immutable fixture data makes duplicate
and concurrent calls safe without idempotency keys.

| Condition | Connect result |
|---|---|
| Exact mock match | Success, `verified=true` |
| Unknown ID or field mismatch | Success, `verified=false` |
| Missing or malformed identity | `InvalidArgument`, provider not called |
| Caller cancellation / deadline | `Canceled` / `DeadlineExceeded` |
| Provider unavailable | `Unavailable` |
| Other technical failure | `Internal` |

Technical errors are sanitized and take precedence over any boolean result.
The caller context is forwarded without retries. Technical failures are tested
with injected provider fakes, rather than special IDs. ScreenSanctions remains
unimplemented.

Example request:

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

Example response: `{"verified":true}`. Changing the ID to `000000000999`
produces `verified=false` without an RPC error (the default false field may be
omitted in protobuf JSON).

Corresponding requests for the other six records (send each object separately):

```json
{"identity":{"idNumber":"000000000002","fullName":"Mock Citizen Two","dateOfBirth":"1985-06-15","idExpiryDate":"2041-06-15","phoneNumber":"0900000000"}}
```

```json
{"identity":{"idNumber":"000000000003","fullName":"Mock Citizen Three","dateOfBirth":"1995-03-20","idExpiryDate":"2042-03-20","phoneNumber":"0900000000"}}
```

```json
{"identity":{"idNumber":"000000000004","fullName":"Mock Citizen Four","dateOfBirth":"2000-09-10","idExpiryDate":"2043-09-10","phoneNumber":"0900000000"}}
```

```json
{"identity":{"idNumber":"000000000005","fullName":"Mock Citizen Five","dateOfBirth":"1988-12-25","idExpiryDate":"2044-12-25","phoneNumber":"0900000000"}}
```

```json
{"identity":{"idNumber":"000000000006","fullName":"Mock Citizen Six","dateOfBirth":"1992-02-29","idExpiryDate":"2044-02-29","phoneNumber":"0900000000"}}
```

```json
{"identity":{"idNumber":"079123456789","fullName":"Nguyen Van A","dateOfBirth":"2000-05-15","idExpiryDate":"2032-05-15","phoneNumber":"0901234567","email":"example@email.com"}}
```

### API 5 — ScreenSanctions

**Purpose**: Check whether the identity requires further sanctions review.

#### Request

```protobuf
ScreenSanctionsRequest {
    identity: IdentityInfo
}
```

#### Response

```protobuf
ScreenSanctionsResponse {
    clear: bool
}
```

### API 6 - AccountService

#### Request

```protobuf
message CreateAccountRequest {
  string application_id = 1;
}
```

#### Response

```protobuf
message CreateAccountResponse {
  string account_id = 1;
  string account_number = 2;
}
```

<br>

## Back Officer APIs

### API 7 — ReviewApplication

**Purpose**: Approve or reject an application in PENDING_HUMAN_REVIEW through a Temporal Update (target).

#### Request

```protobuf
ReviewApplicationRequest {
    application_id: string
    decision: HumanReviewDecision
    optional reason: string
    review_request_id: string
}
```

#### Response

```protobuf
ReviewApplicationResponse {
    application_id: string
    decision_accepted: bool
    status: ApplicationStatus
}
```

<br>

## Customer Progress Checking

### API 8 — GetApplication

**Purpose**: Allow the frontend and back officer to retrieve the current application status.

#### Request

```protobuf
GetApplicationRequest {
  string application_id = 1;
}
```

#### Response

```protobuf
GetApplicationResponse {
  string application_id = 1;
  ApplicationStatus status = 2;
  optional string account_number = 3;
  optional RejectedAtStep rejected_at_step = 4;
}
```

### API 9 — UpdateApplicationStatus

**Purpose**: Allow Temporal Activities to persist workflow progress and business outcomes.

#### Request
```protobuf
message UpdateApplicationStatusRequest {
  string application_id = 1;
  ApplicationStatus expected_status = 2;
  ApplicationStatus new_status = 3;
  optional RejectedAtStep rejected_at_step = 4;
}
```

#### Response
```protobuf
message UpdateApplicationStatusResponse {
  string application_id = 1;
  ApplicationStatus status = 2;
}
```


## Review completion semantics (target)

Main BE authenticates/authorizes the banker; reviewer_id is not a request field.
Only APPROVED/REJECTED are final decisions. Submit an Update with review_request_id
as stable identity. The workflow reserves one decision before yielding; a Main BE
transaction validates and persists audit, request result, and application transition
before the Update completes successfully. Exact replay returns its durable result;
changed payload or a competing decision conflicts. Timeout after acceptance is
uncertain: retry the same ID/payload. decision_accepted does not mean account
creation completed. Response status is the persisted decision result, which may
have progressed by a later GetApplication. Reviewer propagation, audit API, and
exact review RPC error mapping remain implementation decisions.

## Target lifecycle and contract gaps

Both automated clearance and human approval enter APPROVED, then
PENDING_ACCOUNT_CREATION, then COMPLETED after an account is established.
REJECTED records IDENTITY_VERIFICATION or HUMAN_REVIEW as appropriate.
FAILED_TECHNICAL is terminal, not resumable by ordinary retry. Proposed EXPIRED
is a separate terminal customer-inactivity outcome, absent from current protobuf.
Keep existing enum numbers, including legacy PENDING_WORKFLOW_START.

Target training deadlines are seven days from creation for terms and seven days
from persisted acceptance for identity. Main BE enforces them under the parent
lock; new actions at/after deadline return FailedPrecondition. Exact replay of a
committed action remains valid. Expiration Activities recheck committed data so
late Signals cannot expire timely actions. Human review does not auto-expire.
These rules and EXPIRED require later implementation/contract/schema approval.

GetApplication is the persisted business-status view, not Temporal execution or
delivery status. Its fields cannot supply identity, terms, or timestamps needed
by Activities. No owned-data read API exists yet. UpdateApplicationStatus cannot
persist account association, review audit, or detailed processing results.

UpdateApplicationStatus is intended for authorized Activities. Main BE still owns
domain validation and transactions: expected_status does not authorize arbitrary
transitions. Exact replay returns the established result; delayed writes cannot
regress newer state. Detailed replay identities/errors need implementation design.

CreateAccount only receives application_id: authorized identity lookup and
user/customer write ownership remain unresolved. Do not invent fields or permit
cross-service table reads. Account replay must return the same account; uncertain
side effects must be reconciled before technical failure classification.

Internal TermsAccepted/IdentitySubmitted messages carry application ID, stable
event ID, type/version, and committed reference. They are distinct from public
RPCs; exact payload/read/result schemas remain future work. Notification outcome
tracking and permanent review-request replay storage also require design.
