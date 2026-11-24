# Account Opening — Happy-Case Sequence

The Mermaid diagram below transcribes the principal interactions visible in the supplied **Full Creating Account Process (Happy Case)** sequence diagram. It focuses on the happy path; failure handling is documented separately in docs/AI/retry-policy.md

PR 06 starts Temporal after PR 05's atomic identity/pending-start commit. Exact
identity retries at pending-start attempt the same workflow ID again; other
progressed states return unchanged. Accepted startup conditionally advances to
`PENDING_IDENTITY_VERIFICATION`. Startup failure never rolls back identity.
The local worker runs only a deterministic durable wait with no activities.
The business activity interactions below remain future behavior. Detailed startup
errors and completed-execution reconciliation are in `retry-policy.md`.

```mermaid
sequenceDiagram
    autonumber
    actor Customer
    participant MainBE as Main BE
    participant DB as PostgreSQL
    participant Temporal as Temporal Service
    participant Worker as Temporal Worker (Workflow / Activities)
    participant ThirdParty as Third-party API
    participant Account as Account Service
    participant SendGrid as SendGrid

    Customer->>MainBE: StartApplication()
    MainBE->>DB: Insert application (PENDING_TERMS_ACCEPTANCE)
    DB-->>MainBE: Application ID
    MainBE-->>Customer: Application ID

    Customer->>MainBE: AcceptTerms(application_id, term_id)
    MainBE->>DB: Record accepted terms and update status
    DB-->>MainBE: Persisted
    MainBE-->>Customer: Terms accepted

    Customer->>MainBE: SubmitIdentity(application_id, identity)
    MainBE->>DB: Save identity and update application status
    DB-->>MainBE: Persisted
    MainBE->>Temporal: Start onboarding workflow (application_id)
    Temporal-->>MainBE: Workflow accepted
    MainBE-->>Customer: Application submitted / pending verification

    Temporal-->>Worker: Dispatch workflow task
    Worker->>ThirdParty: Verify identity
    ThirdParty-->>Worker: Verified
    Worker->>MainBE: Record identity-verification result / status
    MainBE->>DB: Persist application state
    DB-->>MainBE: Saved

    Worker->>ThirdParty: Screen sanctions
    ThirdParty-->>Worker: Clear
    Worker->>MainBE: Record sanctions result / status
    MainBE->>DB: Persist application state
    DB-->>MainBE: Saved

    Worker->>Account: Create account (application_id, idempotent)
    Account->>DB: Atomic insert with unique application_id
    DB-->>Account: Account record / existing record
    Account-->>Worker: Account created (account number)
    Worker->>MainBE: Record completed application / account result
    MainBE->>DB: Persist completion
    DB-->>MainBE: Saved

    Worker->>SendGrid: Send completion notification
    SendGrid-->>Worker: Notification accepted
    Worker-->>Temporal: Workflow completed
```

## Sanctions Match Requires Human Review

This diagram transcribes the human-review sequence in the supplied image. Message names are descriptive, not new API contracts. Small labels in the image are not fully legible. The pending-review state is written descriptively below rather than assigning an unconfirmed status enum.

```mermaid
sequenceDiagram
    autonumber
    actor Banker
    participant MainBE as Main BE
    participant DB as PostgreSQL
    participant Temporal as Temporal Service
    participant Workflow as Workflow Worker
    participant Activity as Activity Worker
    participant Account as Account Service

    Note over Workflow: Sanctions result is not clear
    Workflow->>Temporal: Schedule activity to record pending human review
    Activity->>Temporal: Poll activity task and execute
    Activity->>MainBE: Record pending human review
    MainBE->>DB: Persist pending human review
    DB-->>MainBE: Saved
    MainBE-->>Activity: Recorded
    Activity-->>Temporal: Activity completed
    Workflow->>Temporal: Poll workflow task and execute
    Note over Temporal,Workflow: Wait durably for banker decision
    Note over Workflow: No active worker task while waiting

    Banker->>MainBE: Review application pending human review
    MainBE-->>Banker: Application details for review
    Banker->>MainBE: Submit review decision
    MainBE->>Temporal: Update workflow with review decision
    Workflow->>Temporal: Poll workflow task for update
    Workflow->>Workflow: Validate pending-review state
    Workflow->>Workflow: Accept decision once atomically
    Workflow-->>Temporal: Update accepted and completed
    Temporal-->>MainBE: Decision accepted
    MainBE-->>Banker: Review recorded for processing

    alt Approved
        Workflow->>Temporal: Schedule approval activity
        Activity->>Temporal: Poll activity task and execute
        Activity->>MainBE: Record approval
        MainBE->>DB: Persist approved state
        DB-->>MainBE: Saved
        MainBE-->>Activity: Approval recorded
        Activity-->>Temporal: Activity completed
        Workflow->>Temporal: Poll workflow task and execute
        Workflow->>Workflow: Continue account-opening process
        Note over Workflow,Account: Resume account creation and notification from the happy-case sequence
    else Rejected
        Workflow->>Temporal: Schedule rejection activity
        Activity->>Temporal: Poll activity task and execute
        Activity->>MainBE: Record rejection
        MainBE->>DB: Persist rejected state
        DB-->>MainBE: Saved
        MainBE-->>Activity: Rejection recorded
        Activity-->>Temporal: Activity completed
        Workflow->>Temporal: Poll workflow task and execute
        Note over Workflow: Continue rejection notification and workflow completion
    end
```

Application-state writes are shown through Main BE to preserve its database ownership. The source image abbreviates these as Activity Worker database writes. Review validation and acceptance must be serialized so concurrent decisions cannot both take effect.
