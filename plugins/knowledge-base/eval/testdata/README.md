# Golden Test Set for Retrieval Evaluation

## What is this?

This directory contains a golden test set for evaluating the quality of CopCon's RAG retrieval pipeline (KnowledgeStore.Search with vector similarity). It provides a standardized benchmark to measure whether the retrieval layer returns the right documents for a given query.

## File Structure

```
eval/testdata/
├── golden_set.jsonl      # 50 test cases, one JSON object per line
├── README.md             # This file
└── fixtures/             # 22 markdown documents used as the knowledge base
    ├── access_control.md
    ├── agent_tool_guide.md
    ├── api_auth_guide.md
    ├── architecture_overview.md
    ├── billing_faq.md
    ├── changelog_2025.md
    ├── compliance_soc2.md
    ├── data_migration_guide.md
    ├── deployment_guide.md
    ├── incident_response.md
    ├── investor_faq.md
    ├── knowledge_management.md
    ├── monitoring_alerting.md
    ├── onboarding_handbook.md
    ├── rate_limiting_policy.md
    ├── refund_policy_v2.md
    ├── release_notes_v3.md
    ├── security_overview.md
    ├── sla_agreement.md
    ├── sso_configuration.md
    ├── troubleshooting_network.md
    └── webhook_integration.md
```

## Golden Set Format

Each line in `golden_set.jsonl` is a JSON object with three fields:

```json
{
  "query": "如何申请退款",
  "relevant_docs": ["refund_policy_v2.md"],
  "category": "frequent"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | The search query, mostly in Chinese with some English |
| `relevant_docs` | string[] | Filenames in `fixtures/` that contain the answer. Empty array for adversarial cases |
| `category` | string | One of: `frequent`, `longtail`, `adversarial`, `temporal` |

## Distribution

| Category | Count | Percentage | Description |
|----------|-------|------------|-------------|
| frequent | 30 | 60% | High-frequency FAQ-style queries with clear document matches |
| longtail | 10 | 20% | Low-frequency, ambiguous, or multi-hop queries spanning 2-3 docs |
| adversarial | 5 | 10% | Queries that should return no results (unrelated to all fixtures) |
| temporal | 5 | 10% | Queries about recent document updates and version changes |

## Quality Gates

The retrieval pipeline should meet these targets against this golden set:

- **Recall@5 >= 0.80**: At least 80% of queries should have all relevant docs in the top 5 results
- **MRR >= 0.75**: The mean reciprocal rank of the first relevant doc should be at least 0.75

## How to Run Evaluation

```bash
# From the project root
cd core && go test -run "TestRetrievalEval" -v

# Or use the eval framework directly
cd core && go run ./eval/... --golden-set ../eval/testdata/golden_set.jsonl --fixtures ../eval/testdata/fixtures/
```

## How to Add New Cases

1. Add fixture documents to `fixtures/` (minimum 200 words, realistic enterprise documentation)
2. Append new lines to `golden_set.jsonl` (one JSON object per line, no trailing comma)
3. Maintain the distribution ratios as the set grows
4. Verify with: `cat golden_set.jsonl | jq . > /dev/null`
5. Check that every filename in `relevant_docs` exists in `fixtures/`

## Design Decisions

**Why Chinese queries?** CopCon's primary user base is Chinese-speaking enterprises. Queries should reflect real user language patterns, including colloquial phrasing and mixed Chinese-English terms.

**Why multi-doc relevance?** Some queries naturally span multiple documents (e.g., security + access control, billing + refund). Multi-doc cases test whether the retriever can surface all relevant context, not just the most obvious match.

**Why adversarial cases?** A good retrieval system must also know when there is no relevant information. Adversarial cases test the system's ability to avoid false positives.

**Why temporal cases?** Enterprise documentation changes frequently. Temporal queries test whether the retriever prioritizes recent content and can match version-specific information.

## Fixture Document Topics

All fixtures are fictional enterprise documentation covering:

- Refund and billing policies
- API authentication and security
- Deployment and architecture
- Onboarding and compliance
- Incident response and monitoring
- SSO and access control
- Knowledge base and agent tooling
- Webhooks and data migration
- Rate limiting and SLA agreements
- Release notes and changelogs
