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
return `Internal`. StartApplication does not start a Temporal workflow.

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
deadlines return `Canceled` and `DeadlineExceeded`. No Temporal workflow starts.

### API 3 — SubmitIdentity

**Purpose**: Submit customer identity details and initiate the asynchronous account-opening workflow.

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
  "application_id": "APP-001",
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
  "application_id": "APP-001",
  "status": "APPLICATION_STATUS_PENDING_WORKFLOW_START"
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

**Purpose**: Approve or Reject an application in PENDING_REVIEW.

#### Request

```protobuf
ReviewApplicationRequest {
    application_id: string
    decision: ReviewDecision
    optional reason: string
    review_request_id: string
    reviewer_id: string
}
```

#### Response

```protobuf
ReviewApplicationResponse {
    application_id: string
    decision_accepted: bool
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
