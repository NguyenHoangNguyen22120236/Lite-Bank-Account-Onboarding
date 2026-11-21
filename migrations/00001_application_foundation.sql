-- +goose Up
-- Goose executes this migration in a transaction by default.
CREATE TYPE application_status AS ENUM (
    'APPLICATION_STATUS_PENDING_TERMS_ACCEPTANCE',
    'APPLICATION_STATUS_PENDING_IDENTITY_SUBMISSION',
    'APPLICATION_STATUS_PENDING_IDENTITY_VERIFICATION',
    'APPLICATION_STATUS_PENDING_SANCTIONS_SCREENING',
    'APPLICATION_STATUS_PENDING_HUMAN_REVIEW',
    'APPLICATION_STATUS_APPROVED',
    'APPLICATION_STATUS_REJECTED',
    'APPLICATION_STATUS_COMPLETED',
    'APPLICATION_STATUS_PENDING_WORKFLOW_START',
    'APPLICATION_STATUS_PENDING_ACCOUNT_CREATION',
    'APPLICATION_STATUS_FAILED_TECHNICAL'
);

CREATE TYPE rejected_at_step AS ENUM (
    'REJECTED_AT_STEP_IDENTITY_VERIFICATION',
    'REJECTED_AT_STEP_HUMAN_REVIEW'
);

CREATE TABLE applications (
    application_id uuid PRIMARY KEY,
    request_id uuid NOT NULL UNIQUE,
    status application_status NOT NULL,
    temporal_workflow_id varchar NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    completed_at timestamptz,
    rejected_at_step rejected_at_step
);

CREATE TABLE terms_acceptances (
    acceptance_id uuid PRIMARY KEY,
    application_id uuid NOT NULL,
    term_id varchar NOT NULL,
    accepted_at timestamptz NOT NULL,
    CONSTRAINT terms_acceptances_application_fk
        FOREIGN KEY (application_id) REFERENCES applications (application_id)
        ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT terms_acceptances_application_term_unique
        UNIQUE (application_id, term_id)
);

CREATE TABLE application_identities (
    application_id uuid PRIMARY KEY,
    id_number varchar NOT NULL,
    full_name varchar NOT NULL,
    date_of_birth date NOT NULL,
    id_expiry_date date NOT NULL,
    phone_number varchar(20) NOT NULL,
    email varchar,
    created_at timestamptz NOT NULL,
    CONSTRAINT application_identities_application_fk
        FOREIGN KEY (application_id) REFERENCES applications (application_id)
        ON UPDATE NO ACTION ON DELETE NO ACTION
);

-- +goose Down
-- Destructive: only roll back on disposable development/test data.
DROP TABLE application_identities;
DROP TABLE terms_acceptances;
DROP TABLE applications;
DROP TYPE rejected_at_step;
DROP TYPE application_status;
