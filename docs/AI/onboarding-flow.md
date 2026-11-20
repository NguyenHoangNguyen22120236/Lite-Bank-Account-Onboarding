# Business Workflow — Bank Account Onboarding

The Mermaid flowchart below is the business workflow reference for implementation. It preserves the User, System, Third-party (Government), and Banker responsibilities shown in the source diagram.

```mermaid
flowchart TB
    subgraph USER["User"]
        direction LR
        START((Start)) --> START_APP["Start application"]
        ACCEPT["Accept the terms"]
        SUBMIT["Submit identity"]
        IDENTITY_DATA["Identity information:<br/>ID number<br/>Full name<br/>Date of birth<br/>ID expiry date<br/>Phone<br/>Email (optional)"]
        IDENTITY_DATA -.->|data| SUBMIT
    end

    subgraph SYSTEM["System"]
        direction LR
        INIT["Initialize application<br/>(start tracking)"]
        VALIDATE["Validate business rules"]
        VALIDATION_RULES["Validation criteria:<br/>Full name present<br/>Valid date of birth<br/>Age ≥ 18<br/>Valid ID number (12 digits)<br/>ID expiry date later than today"]
        PASSED{"Passed?"}
        DECLINED["Declined application"]
        RECORD_ID["Record current state"]
        RECORD_SANCTIONS["Record current state"]
        REVIEW_NEEDED["Need human review"]
        OPEN_ACCOUNT["Open account"]
        NOTIFY["Send notification"]
        END_NODE(((End)))
        VALIDATION_RULES -.->|criteria| VALIDATE
    end

    subgraph THIRD_PARTY["Third-party (Government)"]
        direction LR
        VERIFY["Verify identity"]
        VERIFY_DATA["Verify with mock data (ID)"]
        VERIFIED{"Verified?"}
        SCREEN["Sanctions screening"]
        SCREEN_DATA["Verify with mock data"]
        CLEAR{"Clear?"}
        VERIFY_DATA -.-> VERIFY
        SCREEN_DATA -.-> SCREEN
    end

    subgraph BANKER["Banker"]
        direction LR
        HUMAN_REVIEW["Reviewed by human"]
        APPROVED{"Approved?"}
    end

    START_APP --> INIT
    INIT --> ACCEPT
    ACCEPT --> SUBMIT
    SUBMIT --> VALIDATE
    VALIDATE --> PASSED
    PASSED -->|No| SUBMIT
    PASSED -->|Yes| VERIFY
    VERIFY --> VERIFIED
    VERIFIED -->|No| DECLINED
    VERIFIED -->|Yes| RECORD_ID
    RECORD_ID --> SCREEN
    SCREEN --> CLEAR
    CLEAR -->|Yes| RECORD_SANCTIONS
    RECORD_SANCTIONS --> OPEN_ACCOUNT
    CLEAR -->|No| REVIEW_NEEDED
    REVIEW_NEEDED --> HUMAN_REVIEW
    HUMAN_REVIEW --> APPROVED
    APPROVED -->|Yes| OPEN_ACCOUNT
    APPROVED -->|No| DECLINED
    OPEN_ACCOUNT --> NOTIFY
    DECLINED --> NOTIFY
    NOTIFY --> END_NODE
```
