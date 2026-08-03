#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""
rag-index.py — Index documents from the Grimoire into ChromaDB

Reads documents from a source directory, chunks them, computes embeddings
using sentence-transformers, and stores them in ChromaDB for semantic
retrieval by rag-query.py.

Supports two source types:
  - "kingdom": Markdown, YAML, TOML, Nix files (text-based chunking)
  - "mitre":   STIX 2.1 JSON bundles (technique-based chunking)

Usage:
    python3 rag-index.py \\
        --config /opt/tomb/grimoire/rag/rag-config.json \\
        --source-dir /opt/tomb/grimoire/kingdom-docs \\
        --collection kingdom-docs \\
        --source-type kingdom

    python3 rag-index.py \\
        --config /opt/tomb/grimoire/rag/rag-config.json \\
        --source-dir /opt/tomb/grimoire/mitre-attck \\
        --collection mitre-attck \\
        --source-type mitre
"""

import argparse
import hashlib
import json
import logging
import os
import sys
import time
from pathlib import Path
from typing import Any

from tqdm import tqdm

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%SZ",
)
logger = logging.getLogger("grimoire-index")


# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

def load_config(config_path: str) -> dict:
    """Load RAG configuration from JSON file."""
    with open(config_path, "r") as f:
        return json.load(f)


# ---------------------------------------------------------------------------
# Text Chunking
# ---------------------------------------------------------------------------

class TextChunker:
    """Split text into overlapping chunks for embedding."""

    def __init__(self, chunk_size: int = 512, chunk_overlap: int = 64,
                 min_chunk_size: int = 50, separator: str = "\n\n",
                 fallback_separators: list[str] | None = None):
        self.chunk_size = chunk_size
        self.chunk_overlap = chunk_overlap
        self.min_chunk_size = min_chunk_size
        self.separator = separator
        self.fallback_separators = fallback_separators or ["\n", ". ", " "]

    def chunk(self, text: str) -> list[str]:
        """Split text into chunks, respecting separator boundaries."""
        if len(text) <= self.chunk_size:
            if len(text) >= self.min_chunk_size:
                return [text]
            return []

        chunks = []
        # Split by primary separator first
        segments = text.split(self.separator)

        current_chunk = ""
        for segment in segments:
            segment = segment.strip()
            if not segment:
                continue

            # If adding this segment exceeds chunk_size, finalize current chunk
            candidate = (current_chunk + self.separator + segment).strip() if current_chunk else segment

            if len(candidate) > self.chunk_size and current_chunk:
                chunks.append(current_chunk.strip())
                # Start new chunk with overlap from end of previous
                overlap_text = current_chunk[-self.chunk_overlap:] if len(current_chunk) > self.chunk_overlap else current_chunk
                current_chunk = overlap_text + self.separator + segment
            elif len(candidate) > self.chunk_size and not current_chunk:
                # Single segment exceeds chunk_size; split it further
                sub_chunks = self._split_long_segment(segment)
                chunks.extend(sub_chunks[:-1])
                current_chunk = sub_chunks[-1] if sub_chunks else ""
            else:
                current_chunk = candidate

        if current_chunk.strip() and len(current_chunk.strip()) >= self.min_chunk_size:
            chunks.append(current_chunk.strip())

        return chunks

    def _split_long_segment(self, text: str) -> list[str]:
        """Split a single long segment using fallback separators."""
        for sep in self.fallback_separators:
            parts = text.split(sep)
            if len(parts) > 1:
                chunks = []
                current = ""
                for part in parts:
                    candidate = (current + sep + part).strip() if current else part
                    if len(candidate) > self.chunk_size and current:
                        chunks.append(current.strip())
                        overlap = current[-self.chunk_overlap:] if len(current) > self.chunk_overlap else current
                        current = overlap + sep + part
                    else:
                        current = candidate
                if current.strip() and len(current.strip()) >= self.min_chunk_size:
                    chunks.append(current.strip())
                if chunks:
                    return chunks

        # Last resort: hard split by character count
        chunks = []
        for i in range(0, len(text), self.chunk_size - self.chunk_overlap):
            chunk = text[i:i + self.chunk_size]
            if len(chunk) >= self.min_chunk_size:
                chunks.append(chunk)
        return chunks


# ---------------------------------------------------------------------------
# Document Parsers
# ---------------------------------------------------------------------------

def extract_md_section(text: str) -> str:
    """Extract the first heading from markdown text for section metadata."""
    for line in text.split("\n"):
        line = line.strip()
        if line.startswith("#"):
            return line.lstrip("#").strip()
    return ""


def parse_kingdom_file(file_path: Path) -> list[dict[str, Any]]:
    """Parse a Kingdom doc file into indexable documents."""
    try:
        content = file_path.read_text(encoding="utf-8", errors="replace")
    except Exception as e:
        logger.warning("Failed to read %s: %s", file_path, e)
        return []

    if not content.strip():
        return []

    # Determine doc type from file extension and path
    suffix = file_path.suffix.lower()
    doc_type = "markdown"
    if suffix in (".yaml", ".yml"):
        doc_type = "yaml"
    elif suffix == ".toml":
        doc_type = "toml"
    elif suffix == ".nix":
        doc_type = "nix"

    section = extract_md_section(content)
    relative_path = str(file_path)

    return [{
        "content": content,
        "metadata": {
            "source_file": relative_path,
            "section": section,
            "doc_type": doc_type,
            "source_type": "kingdom",
        },
    }]


def parse_mitre_stix_bundle(file_path: Path) -> list[dict[str, Any]]:
    """Parse a MITRE ATT&CK STIX 2.1 bundle into indexable technique documents."""
    try:
        with open(file_path, "r") as f:
            bundle = json.load(f)
    except Exception as e:
        logger.warning("Failed to parse STIX bundle %s: %s", file_path, e)
        return []

    if not isinstance(bundle, dict) or bundle.get("type") != "bundle":
        logger.warning("File %s is not a STIX bundle", file_path)
        return []

    documents = []
    objects = bundle.get("objects", [])

    for obj in objects:
        obj_type = obj.get("type", "")

        if obj_type == "attack-pattern":
            # This is a technique
            name = obj.get("name", "Unknown")
            description = obj.get("description", "")
            detection = obj.get("x_mitre_detection", "")
            platforms = obj.get("x_mitre_platforms", [])

            # Extract external ID (e.g., T1566)
            external_id = ""
            for ref in obj.get("external_references", []):
                if ref.get("source_name") == "mitre-attack":
                    external_id = ref.get("external_id", "")
                    break

            # Extract kill chain phase (tactic)
            tactics = []
            for phase in obj.get("kill_chain_phases", []):
                if phase.get("kill_chain_name") == "mitre-attack":
                    tactics.append(phase.get("phase_name", ""))

            # Build document text
            text_parts = [
                f"MITRE ATT&CK Technique: {external_id} - {name}",
                "",
                f"Tactic(s): {', '.join(tactics) if tactics else 'Unknown'}",
                f"Platforms: {', '.join(platforms) if platforms else 'All'}",
                "",
                "Description:",
                description,
            ]

            if detection:
                text_parts.extend(["", "Detection:", detection])

            content = "\n".join(text_parts)

            documents.append({
                "content": content,
                "metadata": {
                    "technique_id": external_id,
                    "tactic": ", ".join(tactics),
                    "platforms": ", ".join(platforms),
                    "source_type": "mitre",
                    "source_file": str(file_path),
                },
            })

        elif obj_type == "course-of-action":
            # This is a mitigation
            name = obj.get("name", "Unknown")
            description = obj.get("description", "")

            external_id = ""
            for ref in obj.get("external_references", []):
                if ref.get("source_name") == "mitre-attack":
                    external_id = ref.get("external_id", "")
                    break

            content = f"MITRE ATT&CK Mitigation: {external_id} - {name}\n\n{description}"

            documents.append({
                "content": content,
                "metadata": {
                    "technique_id": external_id,
                    "tactic": "mitigation",
                    "platforms": "",
                    "source_type": "mitre",
                    "source_file": str(file_path),
                },
            })

        elif obj_type == "intrusion-set":
            # This is a threat group
            name = obj.get("name", "Unknown")
            description = obj.get("description", "")
            aliases = obj.get("aliases", [])

            external_id = ""
            for ref in obj.get("external_references", []):
                if ref.get("source_name") == "mitre-attack":
                    external_id = ref.get("external_id", "")
                    break

            alias_str = f"Aliases: {', '.join(aliases)}\n\n" if aliases else ""
            content = f"MITRE ATT&CK Group: {external_id} - {name}\n\n{alias_str}{description}"

            documents.append({
                "content": content,
                "metadata": {
                    "technique_id": external_id,
                    "tactic": "threat-group",
                    "platforms": "",
                    "source_type": "mitre",
                    "source_file": str(file_path),
                },
            })

    logger.info("Parsed %d objects from STIX bundle %s", len(documents), file_path.name)
    return documents


# ---------------------------------------------------------------------------
# Embedding and Indexing
# ---------------------------------------------------------------------------

def compute_doc_id(content: str, metadata: dict) -> str:
    """Generate a deterministic document ID from content and metadata."""
    source = metadata.get("source_file", "")
    tech_id = metadata.get("technique_id", "")
    key = f"{source}:{tech_id}:{content[:200]}"
    return hashlib.sha256(key.encode()).hexdigest()[:24]


def index_documents(config: dict, source_dir: str, collection_name: str,
                    source_type: str) -> None:
    """Index all documents from source_dir into ChromaDB collection."""

    # Import heavy dependencies only when needed
    import chromadb
    from sentence_transformers import SentenceTransformer

    # --- Load embedding model ---
    embed_config = config["embedding"]
    model_name = embed_config["model_name"]
    device = embed_config.get("device", "cpu")
    batch_size = embed_config.get("batch_size", 64)

    logger.info("Loading embedding model: %s (device: %s)", model_name, device)
    model = SentenceTransformer(model_name, device=device)
    logger.info("Embedding model loaded (dimension: %d)", model.get_sentence_embedding_dimension())

    # --- Initialize ChromaDB ---
    chroma_config = config["chromadb"]
    persist_dir = chroma_config["persist_directory"]
    os.makedirs(persist_dir, exist_ok=True)

    logger.info("Initializing ChromaDB at %s", persist_dir)
    client = chromadb.PersistentClient(path=persist_dir)

    # Delete existing collection if re-indexing
    try:
        client.delete_collection(name=collection_name)
        logger.info("Deleted existing collection: %s", collection_name)
    except ValueError:
        pass

    distance_fn = chroma_config.get("distance_function", "cosine")
    collection = client.create_collection(
        name=collection_name,
        metadata={"hnsw:space": distance_fn},
    )
    logger.info("Created collection: %s (distance: %s)", collection_name, distance_fn)

    # --- Discover and parse source files ---
    source_path = Path(source_dir)
    if not source_path.exists():
        logger.error("Source directory does not exist: %s", source_dir)
        sys.exit(1)

    source_type_config = config.get("source_types", {}).get(source_type, {})
    extensions = source_type_config.get("file_extensions", [".md"])

    files = []
    for ext in extensions:
        files.extend(source_path.rglob(f"*{ext}"))

    if not files:
        logger.warning("No files with extensions %s found in %s", extensions, source_dir)
        return

    logger.info("Found %d files to index in %s", len(files), source_dir)

    # --- Parse files into documents ---
    raw_documents = []
    for file_path in tqdm(files, desc="Parsing files", unit="file"):
        if source_type == "mitre":
            raw_documents.extend(parse_mitre_stix_bundle(file_path))
        else:
            raw_documents.extend(parse_kingdom_file(file_path))

    logger.info("Parsed %d raw documents", len(raw_documents))

    if not raw_documents:
        logger.warning("No documents parsed. Nothing to index.")
        return

    # --- Chunk documents ---
    chunk_config = config["chunking"]
    chunker = TextChunker(
        chunk_size=chunk_config["chunk_size"],
        chunk_overlap=chunk_config["chunk_overlap"],
        min_chunk_size=chunk_config["min_chunk_size"],
        separator=chunk_config["separator"],
        fallback_separators=chunk_config.get("fallback_separators"),
    )

    all_chunks = []
    all_metadatas = []
    all_ids = []

    for doc in raw_documents:
        chunks = chunker.chunk(doc["content"])
        for i, chunk_text in enumerate(chunks):
            metadata = dict(doc["metadata"])
            metadata["chunk_index"] = i
            metadata["total_chunks"] = len(chunks)

            doc_id = compute_doc_id(chunk_text, metadata)

            all_chunks.append(chunk_text)
            all_metadatas.append(metadata)
            all_ids.append(doc_id)

    logger.info("Chunked into %d chunks (from %d documents)", len(all_chunks), len(raw_documents))

    if not all_chunks:
        logger.warning("No chunks produced after chunking. Nothing to index.")
        return

    # --- Compute embeddings ---
    logger.info("Computing embeddings (batch_size=%d)...", batch_size)
    start_time = time.time()

    embeddings = model.encode(
        all_chunks,
        batch_size=batch_size,
        show_progress_bar=True,
        normalize_embeddings=embed_config.get("normalize_embeddings", True),
    )

    elapsed = time.time() - start_time
    logger.info("Embeddings computed in %.1f seconds (%.1f chunks/sec)",
                elapsed, len(all_chunks) / elapsed if elapsed > 0 else 0)

    # --- Insert into ChromaDB ---
    # ChromaDB has a batch size limit; insert in batches of 5000
    insert_batch_size = 5000
    total_inserted = 0

    for i in range(0, len(all_chunks), insert_batch_size):
        end = min(i + insert_batch_size, len(all_chunks))
        collection.add(
            documents=all_chunks[i:end],
            embeddings=embeddings[i:end].tolist(),
            metadatas=all_metadatas[i:end],
            ids=all_ids[i:end],
        )
        total_inserted += (end - i)
        logger.info("Inserted %d/%d chunks", total_inserted, len(all_chunks))

    # --- Summary ---
    logger.info("="*60)
    logger.info("INDEXING COMPLETE")
    logger.info("  Collection:    %s", collection_name)
    logger.info("  Documents:     %d", len(raw_documents))
    logger.info("  Chunks:        %d", len(all_chunks))
    logger.info("  Embedding dim: %d", model.get_sentence_embedding_dimension())
    logger.info("  Persist dir:   %s", persist_dir)
    logger.info("="*60)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Index Grimoire documents into ChromaDB for RAG retrieval",
    )
    parser.add_argument(
        "--config",
        default="/opt/tomb/grimoire/rag/rag-config.json",
        help="Path to RAG configuration file (default: /opt/tomb/grimoire/rag/rag-config.json)",
    )
    parser.add_argument(
        "--source-dir",
        required=True,
        help="Directory containing documents to index",
    )
    parser.add_argument(
        "--collection",
        default="kingdom-docs",
        help="ChromaDB collection name (default: kingdom-docs)",
    )
    parser.add_argument(
        "--source-type",
        choices=["kingdom", "mitre"],
        default="kingdom",
        help="Type of source documents (default: kingdom)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Parse and chunk documents but do not index",
    )

    args = parser.parse_args()

    config = load_config(args.config)

    if args.dry_run:
        logger.info("DRY RUN: parsing and chunking only (no indexing)")
        source_path = Path(args.source_dir)
        source_type_config = config.get("source_types", {}).get(args.source_type, {})
        extensions = source_type_config.get("file_extensions", [".md"])

        files = []
        for ext in extensions:
            files.extend(source_path.rglob(f"*{ext}"))

        logger.info("Found %d files", len(files))

        chunker = TextChunker(**{
            k: v for k, v in config["chunking"].items()
            if k in ("chunk_size", "chunk_overlap", "min_chunk_size", "separator", "fallback_separators")
        })

        total_docs = 0
        total_chunks = 0
        for fp in files:
            if args.source_type == "mitre":
                docs = parse_mitre_stix_bundle(fp)
            else:
                docs = parse_kingdom_file(fp)
            total_docs += len(docs)
            for doc in docs:
                chunks = chunker.chunk(doc["content"])
                total_chunks += len(chunks)

        logger.info("Dry run: %d files -> %d documents -> %d chunks", len(files), total_docs, total_chunks)
        return

    index_documents(config, args.source_dir, args.collection, args.source_type)


if __name__ == "__main__":
    main()
