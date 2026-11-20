# High Level Architecture

```mermaid
flowchart LR
    Actor["🧍 Actor"]

    subgraph System["High Level Architecture"]
        direction LR

        Main["Main BE Service<br/>Cloud Run"]
        ThirdParty["Third-party Service<br/>(ID Verify and Sanctions Screening)<br/>Cloud Run"]
        Account["Account Service<br/>Cloud Run"]

        subgraph VPC["VPC"]
            DB[("Cloud SQL<br/>(PostgreSQL)")]
        end

        subgraph WorkerPool["Temporal Worker — Cloud Run Worker Pool"]
            direction TB
            WorkflowWorker["Workflow Worker"]
            ActivityWorker["Activity Worker"]
        end

        Temporal["Temporal Service<br/>Temporal Cloud"]
        SendGrid["SendGrid"]
    end

    Actor -->|HTTPS| Main
    Main <-->|gRPC| ThirdParty
    Main <-->|gRPC| Account
    Main <-->|Read/Write data| DB
    Account <-->|Read/Write data| DB

    Main -->|Start Workflow| Temporal
    WorkerPool -->|Poll Workflow Tasks<br/>Poll Activity Tasks| Temporal

    WorkflowWorker -.- WorkerPool
    ActivityWorker -.- WorkerPool

    ActivityWorker <-->|gRPC| Main
    ActivityWorker <-->|gRPC| Account
    ActivityWorker <-->|gRPC| ThirdParty
    ActivityWorker -->|Send notification event| SendGrid

    style System fill:#fff,stroke:#555,stroke-width:1px
    style VPC fill:#fff,stroke:#777,stroke-dasharray:5 4
    style WorkerPool fill:#fff,stroke:#888
    style WorkflowWorker fill:#fff,stroke:#777,stroke-dasharray:4 3
    style ActivityWorker fill:#fff,stroke:#777,stroke-dasharray:4 3
```

## Component Ownership

| Component | Ownership / Responsibility |
|---|---|
| **Actor** | Initiates onboarding requests and interacts with Main BE over HTTPS. |
| **Main BE Service (Cloud Run)** | Owns the customer-facing application APIs, onboarding application state and application data; reads/writes application data in Cloud SQL; invokes Third-party and Account services over gRPC; starts Temporal workflows. |
| **Third-party Service (Cloud Run)** | Owns ID verification and sanctions screening operations; exposes gRPC operations to callers. |
| **Account Service (Cloud Run)** | Owns bank account creation and account-number generation; reads/writes account data in Cloud SQL; exposes gRPC operations. |
| **Cloud SQL (PostgreSQL)** | Persists application data from Main BE and account data from Account Service. |
| **Temporal Service (Temporal Cloud)** | Owns durable workflow orchestration state and dispatches workflow/activity tasks for workers to poll. |
| **Temporal Worker (Cloud Run Worker Pool)** | Hosts Workflow Worker and Activity Worker; polls Temporal Cloud for workflow and activity tasks. |
| **Workflow Worker** | Executes the workflow orchestration logic. |
| **Activity Worker** | Executes activities that communicate with Main BE, Account Service, and Third-party Service over gRPC and send notification events to SendGrid. |
| **SendGrid** | Receives notification events from the Activity Worker and handles email delivery. |
