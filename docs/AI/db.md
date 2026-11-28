# Database Tables

PostgreSQL schema for this study project. All eight tables share one database and use database foreign keys. This confirmed decision supersedes the earlier separate-database assumption; service write ownership still needs to be respected.

Columns are NOT NULL unless marked nullable. This document records requirements and proposed adjustments; it is not a migration or authorization to alter the database.

## PR 08 target boundary

PR 08 is documentation-only. The current migration and Go behavior remain unchanged.
The target starts Temporal after StartApplication persistence and sends Signals
after terms/identity commits. Business status is separate from startup/Signal
delivery state. Both clearance paths pass through APPROVED. FAILED_TECHNICAL is
terminal. Proposed EXPIRED needs a later additive protobuf/PostgreSQL enum change.
See onboarding-flow.md for transitions and architecture.md for ownership.

## PR 01 migration coverage

`migrations/00001_application_foundation.sql` implements only `applications`, `terms_acceptances`, and `application_identities`, owned by Application Service. The other five tables remain design requirements for later PRs. Shared database placement and future cross-service foreign keys do not authorize cross-service table access.

Goose manages the transactional Up and Down migration. Application code supplies UUIDs and timestamps; there are no defaults, triggers, or extensions. Business validation and state transitions remain outside SQL. Down deletes the application data and is only intended for disposable development/test databases.

## applications

| Column | Type | Constraints / meaning |
|---|---|---|
| application_id | uuid | Parent PK; referenced by child tables. No outgoing FK; the export's unexplained FK label is not implemented. |
| request_id | uuid | NOT NULL UNIQUE; client-generated StartApplicationRequest.request_id. StartApplication resolves duplicate inserts by reading the existing application without overwriting it. |
| status | enum | Follows ApplicationStatus in application.proto. |
| temporal_workflow_id | varchar | Unique; assigned at application creation as the string form of application_id and reused for Temporal start retries. |
| created_at | timestamptz | Creation time. |
| updated_at | timestamptz | Last update time. |
| completed_at | timestamptz | Nullable while processing. PR 08 target populates it for COMPLETED, REJECTED, terminal FAILED_TECHNICAL, and proposed EXPIRED. Existing code does not implement these lifecycle writes. |
| rejected_at_step | enum | Nullable; follows RejectedAtStep in application.proto. |

ApplicationStatus values (APPLICATION_STATUS_ prefix): UNSPECIFIED, PENDING_TERMS_ACCEPTANCE, PENDING_IDENTITY_SUBMISSION, PENDING_IDENTITY_VERIFICATION, PENDING_SANCTIONS_SCREENING, PENDING_HUMAN_REVIEW, APPROVED, REJECTED, COMPLETED, PENDING_WORKFLOW_START, PENDING_ACCOUNT_CREATION, FAILED_TECHNICAL.

RejectedAtStep values (REJECTED_AT_STEP_ prefix): UNSPECIFIED, IDENTITY_VERIFICATION, HUMAN_REVIEW. PR 01 uses named PostgreSQL enums `application_status` and `rejected_at_step`, storing the full protobuf names and excluding UNSPECIFIED. An absent rejection step is SQL NULL.

## terms_acceptances

| Column | Type | Constraints / meaning |
|---|---|---|
| acceptance_id | uuid | PK. |
| application_id | uuid | FK to applications.application_id. |
| term_id | varchar | Identifier supplied by frontend; backend stores the request value. No backend terms-definition table. |
| accepted_at | timestamptz | Acceptance time. |

Confirmed unique constraint: (application_id, term_id). This allows multiple distinct terms per application and prevents duplicates for the same terms. The relationship is consequently one-to-many, adjusting the exported one-to-one label.

PR 04 restricts the AcceptTerms onboarding API to one distinct term per application,
while retaining this broader schema capability. It locks the application row in a
transaction before checking existing acceptances and the state transition. The
acceptance insert and status/timestamp update commit together. Same-term retries
after progression are read-only successes; different terms conflict. An acceptance
already present while status is pending terms is inconsistent and is not repaired
by this API. Other writers must follow the same parent-lock protocol when adding
terms; the per-term unique constraint alone does not enforce the API restriction.

## application_identities

| Column | Type | Constraints / meaning |
|---|---|---|
| application_id | uuid | PK and FK to applications.application_id. |
| id_number | varchar | Submitted identity number; not globally unique across applications. |
| full_name | varchar | Applicant name. |
| date_of_birth | date | Birth date. |
| id_expiry_date | date | Document expiry date. |
| phone_number | varchar(20) | Submitted phone. |
| email | varchar | Nullable. |
| created_at | timestamptz | Creation time. |

At most one identity record per application; repeat applications by a customer are allowed.

Current PR 05 SubmitIdentity locks the parent application row before reading identity or
deciding a transition. The first valid submission inserts identity and advances
`PENDING_IDENTITY_SUBMISSION` to `PENDING_WORKFLOW_START` in one transaction,
using the same application-supplied UTC timestamp for identity creation and the
application update. No schema change is required.

At a progressed status, an exactly matching stored identity is a read-only replay
returning the persisted application status. Equality includes every submitted
field and distinguishes SQL NULL email from an empty email. Changed submissions
conflict. An identity present at either pre-submission status, or a missing identity
at a progressed status, is an invalid state and is not repaired. Current expiry
and age checks apply only to first submissions, so later expiration does not break
durable replay. All identity writers must follow the same parent-lock protocol.
Current PR 06 starts Temporal after this transaction commits. Exact identity replay at
`PENDING_WORKFLOW_START` can retry startup using the stored workflow ID. Following
confirmed startup, a separate parent-row-locked transaction conditionally advances
pending-start to pending identity verification (or `FAILED_TECHNICAL` for confirmed
unsuccessful closure). It preserves every field when status has already advanced.
Identity and its creation timestamp are never changed by startup outcomes.

PR 08 supersedes that target behavior: identity insertion, transition directly to
PENDING_IDENTITY_VERIFICATION, and stable IdentitySubmitted delivery intent commit
together. Startup belongs to StartApplication. Exact replay remains read-only for
business data, while recovery can redeliver its original intent. The existing
pending-start status remains for legacy compatibility; new target applications
start at pending terms and do not use it. Legacy rows need explicit reconciliation.

## Durable delivery and expiration requirements (not migrated)

StartApplication must atomically persist application plus workflow-start intent;
AcceptTerms and SubmitIdentity atomically persist action, status, and Signal intent.
The use-case service owns Transactor.WithinTx. ApplicationRepository remains the
aggregate repository for identity/terms; an infrastructure delivery store supplies
only the operations consumed by those services and recovery. Repository code does
not choose business outcomes or call Temporal.

A future outbox needs stable event ID, application association, event type/version,
minimal committed reference, delivery state, attempt/next-attempt metadata, bounded
claim lease, and acknowledgment time. Enforce uniqueness per logical action and
index pending/available work. Define exact columns, indexes, retention, and claim
SQL in the future schema proposal. Claim briefly, release the transaction before
network delivery, then acknowledge separately. Expired claims are reclaimable;
duplicate delivery is expected. Establish start before dependent Signals.

Persisted creation and terms acceptance timestamps supply customer deadlines:
seven days to accept terms, then seven days to submit identity. Main BE locks the
parent, checks domain deadline/action rules, and writes EXPIRED only if no timely
action committed. New actions at/after deadline fail; exact committed replay stays
valid. Startup/Signal delays do not reset deadlines. No human-review expiry is
introduced. EXPIRED and any delivery tables are absent from the current migration.

## application_reviews

| Column | Type | Constraints / meaning |
|---|---|---|
| review_id | uuid | PK. |
| application_id | uuid | Unique; FK to applications.application_id. |
| reviewer_id | uuid | FK to banker.id; not unique. |
| decision | enum | Follows HumanReviewDecision in application.proto. |
| reason | text | Nullable, matching the optional API reason. |
| reviewed_at | timestamptz | Confirmed correction from exported bigint. |

Exactly one final review for applications requiring human review; other applications have no review record. A banker can review many applications, so the exported banker-to-review one-to-one label is adjusted to one-to-many. Concurrent decisions must be serialized so only one takes effect; retries must not overwrite an accepted decision.

HumanReviewDecision values (HUMAN_REVIEW_DECISION_ prefix): UNSPECIFIED, PENDING,
APPROVED, REJECTED. The PR 08 target persists only APPROVED/REJECTED as final
reviews. Add durable review_request_id handling and the original request/result:
same ID/payload returns the original result; changed payload conflicts. Uniqueness
on application_id alone cannot implement this. Reviewer identity comes from
authenticated context, not a nonexistent protobuf reviewer_id field. Exact schema,
authentication propagation, and review aggregate boundary remain future decisions.
Audit, request result, and resulting application status commit in one Main BE-owned
transaction before the review Update completes successfully.

## user

| Column | Type | Constraints / meaning |
|---|---|---|
| id | uuid | PK. |
| email | varchar | Unique, nullable. |
| phone_number | varchar(20) | Unique. |
| role | enum | CUSTOMER, BANKER, ADMIN. Customer and banker roles are mutually exclusive. |
| created_at | timestamptz | Creation time. |

## customer

| Column | Type | Constraints / meaning |
|---|---|---|
| id | uuid | PK. |
| user_id | uuid | Unique; FK to user.id. |
| created_at | timestamptz | Creation time. |

## banker

| Column | Type | Constraints / meaning |
|---|---|---|
| id | uuid | PK. |
| user_id | uuid | Unique (confirmed); FK to user.id. |
| employee_code | varchar(20) | Unique. |

Unique user_id constraints do not themselves prevent a user from appearing in both customer and banker. Enforcement of role exclusivity requires an implementation decision.

## accounts

| Column | Type | Constraints / meaning |
|---|---|---|
| account_id | uuid | PK. |
| account_number | varchar(30) | Unique. |
| application_id | uuid | Unique; FK to applications.application_id. |
| account_status | enum | ACTIVE, SUSPENDED, CLOSED. These database values are confirmed requirements; account.proto defines no account-status enum. |
| created_at | timestamptz | Creation time. |
| customer_id | uuid | FK to customer.id; not unique. |

Each successful application creates one account. A customer can have many accounts through separate applications. After onboarding checks succeed, create user, customer, and account for a new customer; reuse user/customer and create only the account for an existing customer. Match existing customers by id_number, not phone or email.

Creation of a new user, customer, and account should be atomic to avoid partial records. Retrying account creation for the same application must return the existing account. Concurrent applications with the same id_number must resolve to one customer. The current schema has no unique customer-level id_number mapping; its persistence and concurrency enforcement still need a design. Do not make application_identities.id_number globally unique, because repeat applications are allowed.

## Relationships

| Parent key | Foreign key | Cardinality |
|---|---|---|
| applications.application_id | application_identities.application_id | One to zero or one. |
| applications.application_id | terms_acceptances.application_id | One to many; unique per application and term. |
| applications.application_id | application_reviews.application_id | One to zero or one final review. |
| applications.application_id | accounts.application_id | One to zero or one. |
| user.id | banker.user_id | One to zero or one. |
| user.id | customer.user_id | One to zero or one. |
| banker.id | application_reviews.reviewer_id | One to many. |
| customer.id | accounts.customer_id | One to many. |

PR 01 foreign keys explicitly use ON UPDATE NO ACTION and ON DELETE NO ACTION. Actions for deferred tables remain unspecified; no cascading deletion policy is assumed.

## Remaining decisions and contract differences

- Customer matching uses id_number. Decide how to persist and enforce its unique mapping to a customer, and how changed contact details or conflicting phone/email should be handled.
- Temporal start retries reuse application_id as workflow ID. An already-started workflow must be treated as the existing execution; completed-workflow ID reuse must also be prevented by the configured Temporal policy. A stable ID alone does not prevent a new execution after closure.
- StartApplicationRequest.request_id has durable replay handling through Create and GetByRequestID. ReviewApplicationRequest.review_request_id still requires a durable persistence design; uniqueness on application_id does not by itself implement that contract.
- ReviewApplicationRequest omits reviewer_id; the API document now follows the protobuf. Authentication and trusted reviewer propagation still need design.
- Final review decisions are APPROVED/REJECTED. Which service creates user/customer records remains unresolved; shared placement does not settle write ownership.

Architecture/sequence documents describe the PR 08 target ownership. Protobuf does
not expose completed_at; the target persistence semantics are documented above.

## PR 08 mismatch inventory and future work

| Area | Current contract/schema/code | Target or unresolved dependency |
|---|---|---|
| Start | StartApplicationService uses Create/GetByRequestID without Transactor or Temporal port | Atomically persist start intent; post-commit startup and background recovery |
| Terms | Transactional acceptance/status, no signaling | Atomic Signal intent and post-commit delivery; preserve one-distinct-term rule |
| Identity | Commits PENDING_WORKFLOW_START then invokes WorkflowStarter | Commit pending verification and Signal intent; remove startup ownership later |
| Startup reconciliation | Advances pending-start to verification or technical failure | Must not advance new business progress; reconcile delivery separately |
| Worker | Await(false); Activities/clients/review handler are placeholders, no Activities registered | Implement lifecycle with Signals, Updates, durable timers and Activities |
| Status | Proto, Go and migration share existing values; no EXPIRED | Later additive enum/migration; keep existing numeric values and legacy pending-start |
| Technical failure | Prior documents treated it as recoverable with no completed_at | Target is terminal with completed_at; no ordinary resume |
| Reviews | No migrated review table or request ledger; RPC unimplemented | Durable arbitration/audit and request replay; authenticated reviewer propagation |
| Reads | GetApplication only exposes status/account number/rejection step and is unimplemented | New internal owned-data reads needed for identity, actions, timestamps |
| Results | UpdateApplicationStatus only carries expected/new status and rejection step | Missing review audit, account association and processing result capabilities |
| Accounts | CreateAccount accepts only application_id; service/repository/migrations absent | Resolve owned-data lookup and user/customer creation without cross-table access |
| Delivery | No outbox, claim metadata or recovery process | New schema/ports/recovery and operations policy needed |
| Ownership | Shared database; only three application tables migrated | Account/customer mapping and future cross-service FK behavior still require design |
| Existing executions | Indefinite placeholder workflows and legacy pending-start rows | Explicit replay-compatible migration/reconciliation before new worker rollout |

The API document previously used ReviewDecision, included nonexistent reviewer_id,
and omitted ReviewApplicationResponse.status; PR 08 corrects these descriptions
without changing protobuf. UpdateApplicationStatus.expected_status is only a
concurrency input; Main BE must still enforce valid domain transitions and replay.
No contract currently defines detailed technical-failure information or durable
notification outcomes; decide their storage before implementation.

README remains a historical implementation summary where it describes PR 06, not
the PR 08 target. Before PR 08, the API document's seventh VerifyIdentity fixture
name and HTTP/1 health-check claim conflicted with current code; this PR corrects
both API descriptions. README also contains contradictory HTTP/1 descriptions;
its cleanup is outside this seven-file PR. Linked original Human diagram sources
are historical references, not authorization to change current contracts.
