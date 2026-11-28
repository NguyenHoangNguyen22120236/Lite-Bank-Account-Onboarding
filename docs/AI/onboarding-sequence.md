# Onboarding sequences

Future orchestration target corrected in PR 09 to direct SDK communication.
Only startup is implemented in PR 09; Signals, Updates, Activities, and expiry
remain staged. Signal, Update, internal read, and result message names are
design names, not new protobuf declarations. Current workflow code only waits
indefinitely. See architecture.md and retry-policy.md for ownership and recovery.

## Creation and customer actions

```mermaid
sequenceDiagram
    autonumber
    actor Customer
    participant Main as Main BE
    participant DB as PostgreSQL
    participant T as Temporal
    participant W as Workflow
    Customer->>Main: StartApplication(request_id)
    Main->>DB: Insert application pending terms (autocommit)
    DB-->>Main: Commit
    Main->>T: Ensure started (Workflow ID = application_id)
    T-->>Main: Accepted or existing
    Main->>DB: Read current persisted application
    DB-->>Main: Current business status
    Main-->>Customer: Application ID and persisted status
    T->>W: Execute workflow task
    Note over W: Register handlers; durable wait for terms/deadline
    Customer->>Main: AcceptTerms(application_id, term_id)
    Main->>DB: WithinTx: lock, domain decision, acceptance, status
    DB-->>Main: Commit
    Main->>T: TermsAccepted(stable event ID, committed reference)
    Main-->>Customer: accepted = true
    T->>W: Deliver Signal
    Note over W: Durable wait for identity/deadline
    Customer->>Main: SubmitIdentity(application_id, identity)
    Main->>DB: WithinTx: lock, validate, identity, pending verification
    DB-->>Main: Commit
    Main->>T: IdentitySubmitted(stable event ID, committed reference)
    Main-->>Customer: Application ID and persisted status
    T->>W: Deliver Signal
    Note over W: Process only after both committed actions are established
```

StartApplication succeeds after Temporal accepts or confirms an existing running
execution and Main BE refreshes persisted state. Startup never changes business
status. Terms/identity Signal arrows describe future direct calls, not PR 09.
There is no durable delivery intent or automatic recovery process. Temporal
acceptance is not workflow processing. External calls remain outside transactions.

## Automated processing

```mermaid
sequenceDiagram
    participant W as Workflow
    participant A as Activities
    participant Main as Main BE
    participant Third as Third-party Service
    participant Account as Account Service
    participant Notify as Notification provider
    W->>A: Load committed application and identity
    A->>Main: Read application data (future contract)
    Main-->>A: Persisted data
    W->>A: Verify identity
    A->>Third: VerifyIdentity
    Third-->>A: verified
    alt Identity mismatch
        W->>A: Persist rejection
        A->>Main: Domain-validated conditional rejection
    else Verified
        W->>A: Persist pending sanctions
        A->>Main: Conditional transition
        W->>A: Screen sanctions
        A->>Third: ScreenSanctions
        Third-->>A: clear
        alt Clear
            W->>A: Persist APPROVED
            A->>Main: Conditional approval
        else Match
            W->>A: Persist PENDING_HUMAN_REVIEW
            A->>Main: Conditional transition
            Note over W: Continue through human review sequence
        end
    end
    opt Approved by automation or human review
        W->>A: Persist PENDING_ACCOUNT_CREATION
        A->>Main: Conditional transition
        W->>A: Create account
        A->>Account: CreateAccount(application_id)
        Account-->>A: Same account on retry
        W->>A: Persist completion and account association (future capability)
        A->>Main: Commit COMPLETED
    end
    W->>A: Notify persisted completion or rejection
    A->>Notify: Stable notification identity
    Note over W: Close after required persistence and bounded notification handling
```

Each Main BE write uses a use-case service, domain transition, and service-owned
transaction. Workers never write application tables. Business rejection cannot
fall through into account creation. Technical failures interrupt this happy path.

## Human review Update

```mermaid
sequenceDiagram
    actor Banker
    participant Main as Main BE
    participant T as Temporal
    participant W as Workflow
    participant A as Review persistence Activity
    participant DB as PostgreSQL
    Banker->>Main: ReviewApplication(review_request_id, decision, reason)
    Main->>Main: Authenticate, authorize, validate request
    Main->>T: Update with stable review request ID
    T->>W: Execute Update handler
    W->>W: Check phase; reserve one decision before yielding
    W->>A: Persist accepted review
    A->>Main: Record review with authenticated reviewer context
    Main->>DB: WithinTx: lock, domain decision, audit, request result, status
    DB-->>Main: Commit
    Main-->>A: Durable result
    A-->>W: Persisted
    W-->>T: Update completed with durable decision result
    T-->>Main: Accepted decision and resulting status
    Main-->>Banker: ReviewApplicationResponse
    Note over W: Approval continues account creation; rejection continues notification
```

Only APPROVED/REJECTED are final choices. Competing decisions conflict; exact
request replay returns its durable result. Timeout after acceptance is uncertain:
retry the same ID/payload. Workflow guards arbitrate; Main BE locking/domain rules
and constraints enforce persistence safety. Reviewer propagation/internal audit
contracts remain future work. Required handlers finish before workflow closure.

## Startup retry and future expiration

```mermaid
sequenceDiagram
    actor Customer
    participant Main as Main BE
    participant DB as PostgreSQL
    participant T as Temporal
    Customer->>Main: StartApplication(request_id)
    Main->>DB: Insert application pending terms
    DB-->>Main: Committed
    Note over Main,T: Crash before start or acknowledgment lost
    Main-->>Customer: Error or no response; commit may have succeeded
    Customer->>Main: Retry same request_id
    Main->>DB: Duplicate insert; read existing application
    DB-->>Main: Same application_id and current status
    alt Application terminal
        Main-->>Customer: Persisted result; no Temporal call
    else Application nonterminal
        Main->>T: EnsureStarted with same application_id
        T-->>Main: Started, running, or closed outcome
        Main->>DB: Read current persisted application
        Main-->>Customer: Current status or reconciliation error
    end
```

Without retry, the commit-to-start gap can remain indefinitely. Retained closed
executions are not recreated; startup never writes business status. Nonterminal
history loss needs operational reconciliation.

Future expiration uses a durable timer then a Main BE transaction: lock, recheck
deadline/action, and persist EXPIRED or return the committed action. Startup delay
does not reset deadlines. PR 09 introduces no timers or new enums.
