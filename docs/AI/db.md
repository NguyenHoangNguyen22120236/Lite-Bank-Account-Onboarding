# Database Tables

PostgreSQL schema for this study project. All eight tables share one database and use database foreign keys. This confirmed decision supersedes the earlier separate-database assumption; service write ownership still needs to be respected.

Columns are NOT NULL unless marked nullable. This document records requirements and proposed adjustments; it is not a migration or authorization to alter the database.

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
| completed_at | timestamptz | Nullable while processing and during recoverable FAILED_TECHNICAL states; populated on successful completion and business rejection. |
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

HumanReviewDecision values (HUMAN_REVIEW_DECISION_ prefix): UNSPECIFIED, PENDING, APPROVED, REJECTED. Allowed persisted values for a final review need confirmation.

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
- The protobuf omits reviewer_id from ReviewApplicationRequest, while api/api-design/api-docs.md includes it. Confirm how authenticated banker identity is obtained. The API document also omits some response fields; protobuf remains authoritative.
- Confirm permitted persisted review decisions and which service creates user/customer records. Shared database access does not itself settle write ownership.

Other architecture/sequence documents retain their existing service ownership descriptions. Neither referenced protobuf defines completed_at; its semantics above come from the confirmed project requirement.
