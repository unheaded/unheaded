#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""
rag-query.py — Query ChromaDB for relevant Grimoire documents

Embeds a query string using sentence-transformers, searches ChromaDB
for semantically similar document chunks, and returns results as JSON.

Designed for integration with the Cerydwyn (Ollama/Mistral-7B) RAG pipeline.
The Cerydwyn calls this script, receives context chunks, and augments its
LLM prompt with the retrieved Grimoire knowledge.

Usage:
    # Query a specific collection
    python3 rag-query.py --query "lateral movement in containers" \\
                         --collection mitre-attck

    # Query all collections
    python3 rag-query.py --query "Wotan message bus security" \\
                         --collection all

    # Custom top-k and threshold
    python3 rag-query.py --query "eBPF capabilities" \\
                         --top-k 5 \\
                         --min-score 0.3

    # Output as formatted text instead of JSON
    python3 rag-query.py --query "Shield boundary" --format text

Output (JSON):
    {
      "query": "lateral movement in containers",
      "collection": "mitre-attck",
      "results": [
        {
          "id": "abc123...",
          "content": "MITRE ATT&CK Technique: T1021.004 - SSH...",
          "metadata": {
            "technique_id": "T1021.004",
            "tactic": "lateral-movement",
            "platforms": "Linux, macOS",
            "source_type": "mitre"
          },
          "distance": 0.342,
          "relevance_score": 0.658
        }
      ],
      "total_results": 8,
      "elapsed_ms": 127
    }
"""

import argparse
import json
import logging
import os
import sys
import time
from pathlib import Path
from typing import Any

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%SZ",
)
logger = logging.getLogger("grimoire-query")


# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

def load_config(config_path: str) -> dict:
    """Load RAG configuration from JSON file."""
    with open(config_path, "r") as f:
        return json.load(f)


# ---------------------------------------------------------------------------
# Query Engine
# ---------------------------------------------------------------------------

class GrimoireQueryEngine:
    """Semantic search engine over ChromaDB Grimoire collections."""

    def __init__(self, config: dict):
        self.config = config
        self._model = None
        self._client = None

    @property
    def model(self):
        """Lazy-load the embedding model."""
        if self._model is None:
            from sentence_transformers import SentenceTransformer
            embed_config = self.config["embedding"]
            model_name = embed_config["model_name"]
            device = embed_config.get("device", "cpu")
            logger.info("Loading embedding model: %s", model_name)
            self._model = SentenceTransformer(model_name, device=device)
        return self._model

    @property
    def client(self):
        """Lazy-load the ChromaDB client."""
        if self._client is None:
            import chromadb
            persist_dir = self.config["chromadb"]["persist_directory"]
            if not os.path.exists(persist_dir):
                logger.error("ChromaDB directory not found: %s", persist_dir)
                logger.error("Run rag-index.py first to create the index.")
                sys.exit(1)
            self._client = chromadb.PersistentClient(path=persist_dir)
        return self._client

    def list_collections(self) -> list[str]:
        """List all available ChromaDB collections."""
        collections = self.client.list_collections()
        return [c.name for c in collections]

    def query(self, query_text: str, collection_name: str = "kingdom-docs",
              top_k: int | None = None,
              min_score: float | None = None,
              where: dict | None = None) -> dict[str, Any]:
        """
        Query a ChromaDB collection for relevant documents.

        Args:
            query_text: The natural language query.
            collection_name: ChromaDB collection to search ("all" for multi-collection).
            top_k: Number of results to return (overrides config).
            min_score: Minimum relevance score threshold (overrides config).
            where: Optional metadata filter (ChromaDB where clause).

        Returns:
            Dictionary with query, results, and timing metadata.
        """
        query_config = self.config.get("query", {})
        if top_k is None:
            top_k = query_config.get("top_k", 8)
        if min_score is None:
            min_score = query_config.get("min_relevance_score", 0.25)

        start_time = time.time()

        # Embed the query
        normalize = self.config["embedding"].get("normalize_embeddings", True)
        query_embedding = self.model.encode(
            query_text,
            normalize_embeddings=normalize,
        ).tolist()

        # Determine which collections to search
        if collection_name == "all":
            collection_names = self.list_collections()
            if not collection_names:
                return self._empty_result(query_text, "all", start_time)
        else:
            collection_names = [collection_name]

        # Query each collection and merge results
        all_results = []

        for coll_name in collection_names:
            try:
                collection = self.client.get_collection(name=coll_name)
            except ValueError:
                logger.warning("Collection not found: %s", coll_name)
                continue

            query_params = {
                "query_embeddings": [query_embedding],
                "n_results": top_k,
                "include": ["documents", "metadatas", "distances"],
            }

            if where is not None and collection_name != "all":
                query_params["where"] = where

            results = collection.query(**query_params)

            # Unpack ChromaDB results
            if results["ids"] and results["ids"][0]:
                for i, doc_id in enumerate(results["ids"][0]):
                    distance = results["distances"][0][i]
                    # ChromaDB cosine distance is 1 - similarity for cosine
                    relevance_score = 1.0 - distance

                    if relevance_score < min_score:
                        continue

                    all_results.append({
                        "id": doc_id,
                        "content": results["documents"][0][i],
                        "metadata": results["metadatas"][0][i] if results["metadatas"] else {},
                        "distance": round(distance, 4),
                        "relevance_score": round(relevance_score, 4),
                        "collection": coll_name,
                    })

        # Sort by relevance (highest first) and limit to top_k
        all_results.sort(key=lambda x: x["relevance_score"], reverse=True)
        all_results = all_results[:top_k]

        elapsed_ms = int((time.time() - start_time) * 1000)

        return {
            "query": query_text,
            "collection": collection_name,
            "results": all_results,
            "total_results": len(all_results),
            "elapsed_ms": elapsed_ms,
        }

    def _empty_result(self, query_text: str, collection: str,
                      start_time: float) -> dict[str, Any]:
        elapsed_ms = int((time.time() - start_time) * 1000)
        return {
            "query": query_text,
            "collection": collection,
            "results": [],
            "total_results": 0,
            "elapsed_ms": elapsed_ms,
        }


# ---------------------------------------------------------------------------
# Output Formatting
# ---------------------------------------------------------------------------

def format_json(result: dict) -> str:
    """Format query result as JSON."""
    return json.dumps(result, indent=2, ensure_ascii=False)


def format_text(result: dict) -> str:
    """Format query result as human-readable text."""
    lines = []
    lines.append(f"Query: {result['query']}")
    lines.append(f"Collection: {result['collection']}")
    lines.append(f"Results: {result['total_results']} (in {result['elapsed_ms']}ms)")
    lines.append("=" * 72)

    for i, r in enumerate(result["results"], 1):
        lines.append(f"\n--- Result {i} (score: {r['relevance_score']:.3f}) ---")

        # Show metadata
        meta = r.get("metadata", {})
        source = meta.get("source_file", "unknown")
        source_type = meta.get("source_type", "unknown")
        lines.append(f"Source: {source} [{source_type}]")

        if "technique_id" in meta and meta["technique_id"]:
            lines.append(f"Technique: {meta['technique_id']}")
        if "tactic" in meta and meta["tactic"]:
            lines.append(f"Tactic: {meta['tactic']}")
        if "collection" in r:
            lines.append(f"Collection: {r['collection']}")

        lines.append("")

        # Show content (truncated for readability)
        content = r["content"]
        if len(content) > 800:
            content = content[:800] + "\n... [truncated]"
        lines.append(content)

    return "\n".join(lines)


def format_context(result: dict) -> str:
    """
    Format query result as a context block for LLM prompt augmentation.

    This format is designed to be injected into the Cerydwyn's system prompt
    as retrieved context from the Grimoire.
    """
    if not result["results"]:
        return "<grimoire_context>\nNo relevant documents found.\n</grimoire_context>"

    lines = ["<grimoire_context>"]
    lines.append(f"Query: {result['query']}")
    lines.append(f"Retrieved {result['total_results']} relevant documents.\n")

    for i, r in enumerate(result["results"], 1):
        meta = r.get("metadata", {})
        source = meta.get("source_file", "unknown")
        score = r["relevance_score"]

        lines.append(f"[Document {i}] (relevance: {score:.3f}, source: {source})")
        lines.append(r["content"])
        lines.append("")

    lines.append("</grimoire_context>")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Query the Grimoire knowledge base via ChromaDB semantic search",
    )
    parser.add_argument(
        "--query", "-q",
        required=True,
        help="Natural language query string",
    )
    parser.add_argument(
        "--config",
        default="/opt/tomb/grimoire/rag/rag-config.json",
        help="Path to RAG configuration file",
    )
    parser.add_argument(
        "--collection", "-c",
        default="kingdom-docs",
        help="ChromaDB collection to search (or 'all' for multi-collection)",
    )
    parser.add_argument(
        "--top-k", "-k",
        type=int,
        default=None,
        help="Number of results to return (overrides config)",
    )
    parser.add_argument(
        "--min-score",
        type=float,
        default=None,
        help="Minimum relevance score threshold (0.0-1.0)",
    )
    parser.add_argument(
        "--format", "-f",
        choices=["json", "text", "context"],
        default="json",
        help="Output format: json (machine), text (human), context (LLM prompt)",
    )
    parser.add_argument(
        "--filter-source-type",
        choices=["kingdom", "mitre"],
        default=None,
        help="Filter results by source type",
    )
    parser.add_argument(
        "--list-collections",
        action="store_true",
        help="List available collections and exit",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Suppress log output (only print results)",
    )

    args = parser.parse_args()

    if args.quiet:
        logging.disable(logging.CRITICAL)

    config = load_config(args.config)
    engine = GrimoireQueryEngine(config)

    # List collections mode
    if args.list_collections:
        collections = engine.list_collections()
        if args.format == "json":
            print(json.dumps({"collections": collections}, indent=2))
        else:
            print("Available collections:")
            for c in collections:
                print(f"  - {c}")
        return

    # Build optional where filter
    where = None
    if args.filter_source_type:
        where = {"source_type": {"$eq": args.filter_source_type}}

    # Execute query
    result = engine.query(
        query_text=args.query,
        collection_name=args.collection,
        top_k=args.top_k,
        min_score=args.min_score,
        where=where,
    )

    # Format and print output
    if args.format == "json":
        print(format_json(result))
    elif args.format == "text":
        print(format_text(result))
    elif args.format == "context":
        print(format_context(result))


if __name__ == "__main__":
    main()
