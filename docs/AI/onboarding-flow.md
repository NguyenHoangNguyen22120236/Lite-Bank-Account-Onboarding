# Onboarding lifecycle

This is the PR 08 target, not current implementation. Status labels omit
APPLICATION_STATUS_. EXPIRED requires a future additive contract/schema change.

```mermaid
flowchart TD
    Start[Customer starts application] --> Persist[Main BE commits application and start intent]
    Persist --> Startup[Start workflow with application_id]
    Startup --> Terms[Durably wait for terms or deadline]
    Terms --> Accept[Main BE commits acceptance, status, and Signal intent]
    Accept --> Identity[Durably wait for identity or deadline]
    Identity --> Validate[Main BE validates submitted identity]
    Validate -->|Invalid; no state transition| Identity
    Validate -->|Commit identity, status, and Signal intent| Verify[Activity verifies identity]
    Verify -->|Not verified| Reject[Main BE records REJECTED]
    Verify -->|Verified| Screen[Activity screens sanctions]
    Screen -->|Clear| Approve[Main BE records APPROVED]
    Screen -->|Match| Review[Durably wait for human review Update]
    Review -->|Approve and persist audit| Approve
    Review -->|Reject and persist audit| Reject
    Approve --> Account[Record pending account creation; create account idempotently]
    Account --> Complete[Main BE records COMPLETED]
    Terms -->|Deadline; Main BE confirms action absent| Expire[Main BE records EXPIRED]
    Identity -->|Deadline; Main BE confirms action absent| Expire
    Complete --> Notify[Notification Activity]
    Reject --> Notify
    Notify --> End[Close after required persistence]
    Expire --> End
    Technical[Unrecoverable technical failure after reconciliation] --> Failed[Main BE records FAILED_TECHNICAL]
    Failed --> End
```

Invalid customer input leaves the action available for correction. Identity
mismatch and banker rejection are business outcomes. Technical retries happen
before terminal failure. Notification failure never reverses completion/rejection.

```mermaid
stateDiagram-v2
    [*] --> PENDING_TERMS_ACCEPTANCE
    PENDING_TERMS_ACCEPTANCE --> PENDING_IDENTITY_SUBMISSION: Terms committed
    PENDING_IDENTITY_SUBMISSION --> PENDING_IDENTITY_VERIFICATION: Identity committed
    PENDING_IDENTITY_VERIFICATION --> REJECTED: Identity mismatch
    PENDING_IDENTITY_VERIFICATION --> PENDING_SANCTIONS_SCREENING: Verified
    PENDING_SANCTIONS_SCREENING --> APPROVED: Clear
    PENDING_SANCTIONS_SCREENING --> PENDING_HUMAN_REVIEW: Match
    PENDING_HUMAN_REVIEW --> APPROVED: Approval persisted
    PENDING_HUMAN_REVIEW --> REJECTED: Rejection persisted
    APPROVED --> PENDING_ACCOUNT_CREATION
    PENDING_ACCOUNT_CREATION --> COMPLETED: Account established
    PENDING_TERMS_ACCEPTANCE --> EXPIRED: Terms deadline
    PENDING_IDENTITY_SUBMISSION --> EXPIRED: Identity deadline
    COMPLETED --> [*]
    REJECTED --> [*]
    EXPIRED --> [*]
    FAILED_TECHNICAL --> [*]
```

FAILED_TECHNICAL is reachable from nonterminal processing after unrecoverable
technical failure and external-effect reconciliation; repeated arrows are omitted.
Terminal states have no normal outgoing transition. An existing account must be
reconciled to completion, not failed because its response was lost.

Terms expire seven days after creation; identity expires seven days after persisted
acceptance. Main BE checks deadlines during action transactions with explicit time
passed to domain rules. New actions at or after the deadline fail; exact replay of
committed actions remains valid. Expiration locks and rechecks persisted actions,
so delayed Signals cannot expire an action committed on time. Human review does
not automatically expire or reject; overdue review requires operational alerts.

PENDING_WORKFLOW_START is retained for legacy compatibility, not used for new
target applications. Startup delivery is separate from business progress.
APPROVED is not COMPLETED; notification delivery is separate from both.
Normal workflow closure follows terminal persistence. Abnormal closure requires
reconciliation rather than inference of a business outcome.