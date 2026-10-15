## ADDED Requirements

### Requirement: Scoped composable optimization tools
The system SHALL expose query rewriting, HyDE and bounded multi-hop as Agent tools using the current authorized knowledge scope and returning actual retrieved evidence.

#### Scenario: Prompt attempts to expand scope
- **WHEN** an optimization query requests content from a different tenant or unauthorized knowledge base
- **THEN** all retrieval remains restricted to authenticated authorized sources

### Requirement: Graph retriever integration
The system SHALL adapt the existing graph query repository into the RAG Retriever and return actual linked chunks with scoped authorization and visible dependency errors.

#### Scenario: Linked graph evidence
- **WHEN** authorized graph entities link to document chunks
- **THEN** graph retrieval returns those real chunks and excludes chunks outside the knowledge namespace
