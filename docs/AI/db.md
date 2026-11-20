# Database Tables

This document describes the supplied database diagram. It is a reference for discussion, not a migration or authorization to change the schema. Some labels are small, so uncertain types and constraints are called out below.

## Table overview

| Table | Purpose |
|---|---|
| `applications` | Tracks an onboarding application and its processing state. |
| `terms_acceptances` | Records the terms accepted for an application. |
| `application_identities` | Stores identity information submitted for an application. |
| `application_reviews` | Records human review decisions. |
| `user` | Stores shared user contact and role information. |
| `customer` | Represents a customer linked to a user. |
| `banker` | Represents a banker linked to a user. |
| `accounts` | Stores bank accounts and links them to applications and customers. |

## `applications`

| Column | Diagram type | Description |
|---|---|---|
| `application_id` | `uuid` | Primary identifier for an onboarding application. |
| `status` | `enum` | Current application state. Enum values are not shown. |
| `temporal_workflow_id` | `varchar` | Identifies the Temporal workflow associated with the application. |
| `created_at` | `timestamptz` | When the application was created. |
| `updated_at` | `timestamptz` | When the application was last updated. |
| `completed_at` | `timestamptz` | When processing completed. Nullability is not clear in the image. |
| `rejected_at_step` | `enum?` | Step at which the application was rejected. The exact label and type need confirmation. |

## `terms_acceptances`

| Column | Diagram type | Description |
|---|---|---|
| `acceptance_id` | `uuid` | Primary identifier for an acceptance record. |
| `application_id` | `uuid` | Application for which terms were accepted. References `applications.application_id`. |
| `term_id` | `varchar` | Identifier of the accepted terms. A terms-definition table is not shown. |
| `accepted_at` | `timestamptz` | When the terms were accepted. |

## `application_identities`

| Column | Diagram type | Description |
|---|---|---|
| `application_id` | `uuid` | Identifies the application and links to `applications.application_id`. Primary key. |
| `id_number` | `varchar` | Submitted identity-document number. |
| `full_name` | `varchar` | Applicant's full name. |
| `date_of_birth` | `date` | Applicant's date of birth. |
| `id_expiry_date` | `date` | Identity-document expiry date. |
| `phone_number` | `varchar(20)` | Applicant's phone number. |
| `email` | `varchar?` | Applicant's email address. Exact type length is unclear. |
| `created_at` | `timestamptz` | When the identity record was created. |

## `application_reviews`

| Column | Diagram type | Description |
|---|---|---|
| `review_id` | `uuid` | Primary identifier for a review record. |
| `application_id` | `uuid` | Unique key, Reviewed application. References `applications.application_id`. |
| `reviewer_id` | `uuid` | Reviewer identifier. A relationship to `banker` is drawn, but the apparent type differs from `banker.id`. |
| `decision` | `enum` | Review outcome. Exact enum values are not shown. |
| `reason` | `text` | Explanation for the review decision. |
| `reviewed_at` | `bigint` | Review time. The timestamp representation and unit need confirmation. |


## `user`

| Column | Diagram type | Description |
|---|---|---|
| `id` | `uuid` | Primary user identifier. |
| `email` | `varchar?` | User email address. Exact type length is unclear. |
| `phone_number` | `varchar(20)` | User phone number. |
| `role` | `enum` | User role. Exact values are not shown. |
| `created_at` | `timestamptz` | When the user record was created. |

## `customer`

| Column | Diagram type | Description |
|---|---|---|
| `id` | `uuid` | Primary customer identifier. |
| `user_id` | `uuid` | Links the customer to a user in the diagram. This apparent type conflicts with `user.id`, which is `uuid`. |
| `created_at` | `timestamptz` | When the customer record was created. |

## `banker`

| Column | Diagram type | Description |
|---|---|---|
| `id` | `uuid` | Primary banker identifier. |
| `user_id` | `uuid` | User associated with the banker. References `user.id`. |
| `employee_code` | `varchar(20)` | Banker's employee identifier. |

## `accounts`

| Column | Diagram type | Description |
|---|---|---|
| `account_id` | `uuid` | Primary account identifier. |
| `account_number` | `varchar(20)` | Bank account number. The diagram appears to mark it as unique. |
| `application_id` | `uuid` | Application that produced the account. The diagram appears to mark it as unique. |
| `account_status` | `enum` | Current account state. Exact values are not shown. |
| `created_at` | `timestamptz` | When the account was created. |
| `customer_id` | `uuid` | Customer associated with the account. References `customer.id`. |

## Relationships

| Parent | Related records | Interpretation |
|---|---|---|
| `applications` | `terms_acceptances` | Each acceptance belongs to an application. No per-terms uniqueness rule is confirmed. |
| `applications` | `application_identities` | At most one identity record per application if the displayed primary key is confirmed. |
| `applications` | `application_reviews` | Each review belongs to an application. Whether multiple reviews are allowed needs confirmation. |
| `banker` | `application_reviews` | Reviews identify a banker as reviewer. Confirm the referenced key and compatible types. |
| `user` | `customer`, `banker` | Customer and banker records link to a user. One-to-one relationships require uniqueness on `user_id`, which is not confirmed. |
| `applications` | `accounts` | An account originates from an application. At most one account per application requires the displayed uniqueness to be confirmed. |
| `customer` | `accounts` | Each account identifies a customer. No one-account-per-customer restriction is established here. |
