# Zhen AI — Local RAG Context Provider

Zhen is running at http://localhost:20103 with 594K indexed chunks.

## For Claude Code agents:

Before working on any task, query Zhen for context:
```bash
curl -s http://localhost:20103/api/v1/context \
  -H "Content-Type: application/json" \
  -d '{"task": "describe your task here", "k": 10}'
```

This returns the 10 most relevant code/doc chunks from:
- Unheaded codebase (385K LOC)
- 16 GitHub repos (k8s, prometheus, tokio, etc.)
- 9,739 IETF RFCs
- 1,649 research papers
- Kingdom skill files (18+ skills)

## Available endpoints:
- POST /api/v1/query — RAG question answering
- POST /api/v1/search — Semantic search (no generation)
- POST /api/v1/context — Context retrieval for Claude agents
- GET /api/v1/skills — List all Kingdom skills
- GET /api/v1/skill/<name> — Get specific skill content
- GET /api/v1/stats — Index statistics
- GET /api/v1/corpus/stats — Corpus breakdown by ring
