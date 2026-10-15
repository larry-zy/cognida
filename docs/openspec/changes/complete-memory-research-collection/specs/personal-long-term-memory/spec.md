## ADDED Requirements

### Requirement: Private persistent memory lifecycle
The system SHALL persist explicit user facts and preferences with tenant/user scope, idempotent creation, version-checked updates, cursor listing and deletion that removes stored content.

#### Scenario: Memory isolation and forgetting
- **WHEN** a user creates a memory, another user or tenant queries it, and its owner later deletes it
- **THEN** only its owner can read or change it and subsequent recall excludes deleted content

### Requirement: Bounded cross-session recall
The system SHALL recall relevant private facts and preferences into new Agent conversations as untrusted user context, with bounded content and no shared answer-cache leakage.

#### Scenario: New conversation after correction
- **WHEN** an owner corrects a preference and starts a new conversation
- **THEN** the Agent receives the current preference, not a stale cached answer or another user's memory
