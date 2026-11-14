## **Service:** ApplicationService — Main BE

### API 1 — StartApplication

**Purpose:** Initialize a new application and begin tracking it.

#### Request

```protobuf
StartApplicationRequest {
}
```

#### Response

```protobuf
StartApplicationResponse {
  application_id: string
}
```

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

SubmitIdentityResponse {
application_id: string
status: ApplicationStatus
}

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
  "status": "PENDING_VERIFICATION"
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

<br>

## Back Officer APIs

### API 7 — ReviewApplication

**Purpose**: Approve or Reject an application in PENDING_REVIEW.

#### Request

```protobuf
ReviewApplicationRequest {
    application_id: string
    decision: ReviewDecision
}
```

#### Response

```protobuf
ReviewApplicationResponse {
    application_id: string
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
  optional string account_number;
}
```
