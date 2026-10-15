## ADDED Requirements

### Requirement: Evidence-based research execution
The system SHALL execute bounded planning, scoped multi-source retrieval, synthesis and citation checks through a usable authenticated research API and page.

#### Scenario: Research with supporting evidence
- **WHEN** a user requests research with accessible knowledge sources
- **THEN** the report contains actual subquestions, source identifiers, cited findings and explicit verification limitations

### Requirement: Truthful terminal states
The system SHALL propagate timeout/cancellation, report source failures, reject unsupported citations and return the completed report in the terminal SSE event.

#### Scenario: Empty or failing sources
- **WHEN** all selected sources fail or return no evidence
- **THEN** the workflow returns a visible failure instead of an empty successful report
