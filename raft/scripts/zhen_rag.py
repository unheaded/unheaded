#!/usr/bin/env python3
"""
Zhen RAG Pipeline — Retrieval-Augmented Generation for Unheaded
Combines FAISS retrieval with Mistral-7B inference via llama.cpp
and Claude API handoff for prompts exceeding local context window.
"""
import json
import os
import time
import requests
import faiss
import numpy as np
from sentence_transformers import SentenceTransformer
from pathlib import Path


class RAGPipeline:
    def __init__(self, index_dir, corpus_file, inference_url="http://localhost:20100"):
        self.inference_url = inference_url
        self.index_dir = Path(index_dir)
        self.corpus_file = Path(corpus_file)

        # Context window config (updated by benchmark experiment)
        self.local_max_tokens = int(os.environ.get('ZHEN_LOCAL_MAX_TOKENS', '2048'))

        # Load embedding model
        print("Loading embedding model...")
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')

        # Load FAISS index
        print("Loading FAISS index...")
        # Use active.index symlink (points to ring1 or combined)
        active_index = self.index_dir / 'active.index'
        index_path = active_index if active_index.exists() else self.index_dir / 'ring1.index'
        self.index = faiss.read_index(str(index_path))
        print(f"  Index: {index_path.name} → {index_path.resolve().name}")

        # Load ID map (can be list or dict depending on how it was saved)
        active_ids = self.index_dir / 'active_ids.json'
        ids_path = active_ids if active_ids.exists() else self.index_dir / 'ring1_ids.json'
        with open(ids_path, 'r') as f:
            raw_ids = json.load(f)
            if isinstance(raw_ids, list):
                self.id_map = {str(i): v for i, v in enumerate(raw_ids)}
            else:
                self.id_map = raw_ids

        # Load corpus for content retrieval
        # Strategy: load ring_all.jsonl (combined corpus with full content)
        # Falls back to ring1 + metadata if ring_all doesn't exist
        print("Loading corpus...")
        self.corpus = {}
        corpus_dir = self.corpus_file.parent

        ring_all = corpus_dir / 'ring_all.jsonl'
        if ring_all.exists():
            count = 0
            with open(ring_all, 'r', encoding='utf-8', errors='ignore') as f:
                for line in f:
                    try:
                        chunk = json.loads(line)
                        cid = chunk.get('id', '')
                        content = chunk.get('content', '')
                        if cid and content:
                            self.corpus[cid] = {
                                'content': content,
                                'source': chunk.get('source', ''),
                                'type': chunk.get('type', 'unknown'),
                            }
                            count += 1
                    except (json.JSONDecodeError, KeyError):
                        continue
            print(f"  All rings: {count:,} chunks (full content)")

            # Wikipedia: build line-offset index for on-demand lookup.
            # 29GB corpus is too large for RAM. Instead, index byte offsets
            # per chunk ID, then seek+read on retrieval.
            wiki_path = corpus_dir / 'wikipedia.jsonl'
            wiki_idx_path = corpus_dir / 'wikipedia_offsets.json'
            if wiki_path.exists():
                self.wiki_path = wiki_path
                if wiki_idx_path.exists():
                    print("  Loading Wikipedia offset index...")
                    with open(wiki_idx_path) as f:
                        self.wiki_offsets = json.load(f)
                    print(f"  Wikipedia: {len(self.wiki_offsets):,} chunks indexed (on-demand)")
                else:
                    print("  Building Wikipedia offset index (one-time)...")
                    self.wiki_offsets = {}
                    with open(wiki_path, 'r', encoding='utf-8', errors='ignore') as f:
                        offset = 0
                        for line in f:
                            try:
                                cid = json.loads(line).get('id', '')
                                if cid:
                                    self.wiki_offsets[cid] = offset
                            except (json.JSONDecodeError, KeyError):
                                pass
                            offset += len(line.encode('utf-8'))
                    with open(wiki_idx_path, 'w') as f:
                        json.dump(self.wiki_offsets, f)
                    print(f"  Wikipedia: {len(self.wiki_offsets):,} chunks indexed (saved)")
            else:
                self.wiki_path = None
                self.wiki_offsets = {}
        else:
            # Fallback: Ring 1 full content + Ring 2-4 metadata
            ring1_path = corpus_dir / 'ring1.jsonl'
            if ring1_path.exists():
                count = 0
                with open(ring1_path, 'r', encoding='utf-8', errors='ignore') as f:
                    for line in f:
                        try:
                            chunk = json.loads(line)
                            if 'content' in chunk:
                                self.corpus[chunk['id']] = {
                                    'content': chunk['content'],
                                    'source': chunk.get('source', ''),
                                    'type': chunk.get('type', 'unknown'),
                                }
                                count += 1
                        except (json.JSONDecodeError, KeyError):
                            continue
                print(f"  Ring 1: {count:,} chunks (full content)")

            combined_meta = self.index_dir / 'combined_corpus.jsonl'
            if combined_meta.exists():
                count = 0
                with open(combined_meta, 'r', encoding='utf-8', errors='ignore') as f:
                    for line in f:
                        try:
                            chunk = json.loads(line)
                            cid = chunk.get('id', '')
                            if cid and cid not in self.corpus:
                                self.corpus[cid] = {
                                    'content': f"[Source: {chunk.get('source', 'unknown')}]",
                                    'source': chunk.get('source', ''),
                                    'type': chunk.get('type', 'unknown'),
                                }
                                count += 1
                        except (json.JSONDecodeError, KeyError):
                            continue
                print(f"  Ring 2-4: {count:,} chunks (metadata only)")

        # Memory store (loaded from DB on first access)
        self._memories = []

        print(f"RAG Pipeline ready: {len(self.id_map)} vectors, {len(self.corpus)} chunks")

    def estimate_tokens(self, text):
        """Rough token estimate: ~4 chars per token for English."""
        return len(text) // 4 + 50  # +50 for system prompt overhead

    def retrieve(self, query, k=5):
        """Retrieve top-k chunks from FAISS index"""
        query_embedding = self.embedding_model.encode(query, convert_to_numpy=True)
        query_embedding = query_embedding.astype('float32').reshape(1, -1)

        distances, indices = self.index.search(query_embedding, k)

        retrieved = []
        for idx, distance in zip(indices[0], distances[0]):
            if idx < 0:
                continue
            chunk_id = self.id_map.get(str(idx), self.id_map.get(str(int(idx))))
            if not chunk_id:
                continue
            if chunk_id in self.corpus:
                data = self.corpus[chunk_id]
                retrieved.append({
                    'id': chunk_id,
                    'content': data['content'],
                    'source': data['source'],
                    'type': data['type'],
                    'distance': float(distance),
                })
            elif chunk_id in getattr(self, 'wiki_offsets', {}):
                # On-demand Wikipedia lookup: seek to byte offset, read line
                data = self._fetch_wiki_chunk(chunk_id)
                if data:
                    retrieved.append({
                        'id': chunk_id,
                        'content': data['content'],
                        'source': data['source'],
                        'type': 'wikipedia',
                        'distance': float(distance),
                    })
        return retrieved

    def _fetch_wiki_chunk(self, chunk_id):
        """Fetch a single Wikipedia chunk by seeking to its byte offset."""
        if not self.wiki_path or chunk_id not in self.wiki_offsets:
            return None
        try:
            with open(self.wiki_path, 'r', encoding='utf-8', errors='ignore') as f:
                f.seek(self.wiki_offsets[chunk_id])
                line = f.readline()
                chunk = json.loads(line)
                return {
                    'content': chunk.get('content', ''),
                    'source': chunk.get('source', ''),
                }
        except Exception:
            return None

    def generate(self, query, context_chunks, file_content=None, history=None):
        """Generate response using local Mistral-7B via llama.cpp.

        Args:
            query: The user question
            context_chunks: Retrieved RAG context
            file_content: Optional file content appended to the prompt
            history: List of prior turns [{'role': 'user'|'assistant', 'content': '...'}, ...]
        """
        # Use more chunks when context window allows
        max_chunks = 3 if self.local_max_tokens <= 2048 else 5
        chunk_limit = 1500 if self.local_max_tokens <= 2048 else 2500

        context = "\n\n---\n\n".join([
            f"[Source: {c['source']}]\n{c['content'][:chunk_limit]}"
            for c in context_chunks[:max_chunks]
        ])

        if file_content:
            context += f"\n\n---\n\n[Uploaded file content]\n{file_content}"

        system = """You are Zhen (真爱), the AI champion of the Unheaded Kingdom.
You are an expert on Unheaded's architecture, services, protocols, and codebase.
You also have knowledge from technical documentation, RFCs, research papers, GitHub repositories, and Wikipedia.
Use the following retrieved context to answer accurately.
If the context contains relevant information, use it. If not, answer from general knowledge but note the source.
When the user refers to previous answers (e.g. "elaborate on 2 and 4", "tell me more about that"), use the conversation history."""

        # Build Mistral multi-turn prompt
        # Correct format: <s>[INST] sys+ctx+Q1 [/INST] A1</s>[INST] Q2 [/INST] A2</s>[INST] Q3 [/INST]
        if history:
            # Build history pairs, trimmed to budget
            history_budget = int(self.local_max_tokens * 1.5)  # chars
            pairs = []  # [(user_msg, assistant_msg), ...]
            i = 0
            while i < len(history) - 1:
                if history[i]['role'] == 'user' and history[i+1]['role'] == 'assistant':
                    pairs.append((history[i]['content'], history[i+1]['content']))
                    i += 2
                else:
                    i += 1

            # Include as many recent pairs as fit
            included_pairs = []
            total_len = 0
            for user_msg, asst_msg in reversed(pairs):
                pair_len = len(user_msg) + len(asst_msg) + 30  # overhead for tags
                if total_len + pair_len > history_budget:
                    break
                included_pairs.insert(0, (user_msg, asst_msg))
                total_len += pair_len

            if included_pairs:
                # First turn includes system + context
                first_user, first_asst = included_pairs[0]
                prompt = f"<s>[INST] {system}\n\nCONTEXT:\n{context}\n\nQUESTION: {first_user} [/INST]{first_asst}</s>"

                # Middle turns
                for user_msg, asst_msg in included_pairs[1:]:
                    prompt += f"[INST] {user_msg} [/INST]{asst_msg}</s>"

                # Current question
                prompt += f"[INST] {query} [/INST]"
            else:
                # History exists but too long to include — fall through to no-history path
                prompt = f"<s>[INST] {system}\n\nCONTEXT:\n{context}\n\nQUESTION: {query} [/INST]"
        else:
            prompt = f"<s>[INST] {system}\n\nCONTEXT:\n{context}\n\nQUESTION: {query} [/INST]"

        # Final truncation safety net
        estimated_tokens = self.estimate_tokens(prompt)
        if estimated_tokens > self.local_max_tokens * 0.85:
            budget = int(self.local_max_tokens * 3.2)
            if len(prompt) > budget:
                # Rebuild with shorter context
                context = context[:int(budget * 0.4)] + "\n[...truncated to fit context window]"
                prompt = f"<s>[INST] {system}\n\nCONTEXT:\n{context}\n\nQUESTION: {query} [/INST]"

        try:
            response = requests.post(
                f"{self.inference_url}/v1/completions",
                json={
                    "prompt": prompt,
                    "max_tokens": 500,
                    "temperature": 0.3,
                    "top_p": 0.9,
                    "stop": ["[INST]", "</s>"],
                },
                timeout=60,
            )

            if response.status_code == 200:
                result = response.json()
                return {
                    'answer': result['choices'][0]['text'].strip(),
                    'tokens_used': result.get('usage', {}).get('completion_tokens', 0),
                    'model': 'mistral-7b',
                }
            else:
                return {'answer': f"Inference error: {response.status_code}", 'tokens_used': 0, 'model': 'mistral-7b'}

        except requests.exceptions.ConnectionError:
            return {'answer': "Error: Inference server not reachable on port 20100", 'tokens_used': 0, 'model': 'mistral-7b'}
        except requests.exceptions.Timeout:
            return {'answer': "Error: Inference timed out (60s)", 'tokens_used': 0, 'model': 'mistral-7b'}

    def query(self, question, file_content=None, history=None):
        """Full RAG query: retrieve + generate"""
        retrieved = self.retrieve(question, k=5)
        result = self.generate(question, retrieved, file_content=file_content, history=history)
        return {
            'question': question,
            'retrieved': retrieved,
            'answer': result['answer'],
            'tokens_used': result.get('tokens_used', 0),
            'model': result.get('model', 'mistral-7b'),
        }

    def add_to_corpus(self, text, source="user"):
        """Add new text to the corpus and FAISS index (teach endpoint).

        Chunks the text, embeds it, and adds to the live FAISS index.
        No restart needed — FAISS supports index.add().
        """
        # Simple chunking: split by double newlines, then by size
        chunks = []
        for para in text.split('\n\n'):
            para = para.strip()
            if not para:
                continue
            # Split long paragraphs into ~500 char chunks
            while len(para) > 500:
                # Find a good split point
                split_at = para.rfind('. ', 0, 500)
                if split_at < 100:
                    split_at = 500
                chunks.append(para[:split_at + 1].strip())
                para = para[split_at + 1:].strip()
            if para:
                chunks.append(para)

        if not chunks:
            return {'added': 0}

        # Embed chunks
        embeddings = self.embedding_model.encode(chunks, convert_to_numpy=True)
        embeddings = embeddings.astype('float32')

        # Add to FAISS index
        start_idx = self.index.ntotal
        self.index.add(embeddings)

        # Add to corpus and id_map
        corpus_dir = self.corpus_file.parent
        added = 0
        with open(corpus_dir / 'ring_all.jsonl', 'a', encoding='utf-8') as f:
            for i, chunk_text in enumerate(chunks):
                cid = f"taught_{source}_{int(time.time())}_{i}"
                idx = start_idx + i
                self.id_map[str(idx)] = cid
                self.corpus[cid] = {
                    'content': chunk_text,
                    'source': f'taught:{source}',
                    'type': 'taught',
                }
                f.write(json.dumps({'id': cid, 'content': chunk_text, 'source': f'taught:{source}', 'type': 'taught'}) + '\n')
                added += 1

        return {'added': added, 'chunks': [c[:100] + '...' if len(c) > 100 else c for c in chunks]}


def main():
    index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
    corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'

    rag = RAGPipeline(index_dir, corpus_file)

    test_queries = [
        "What is Unheaded?",
        "How does the eBPF layer work in Unheaded?",
        "What are the core services in Unheaded?",
        "What is the Wotan message bus?",
        "How does the Monad wire format work?",
    ]

    for q in test_queries:
        print(f"\n{'='*60}")
        result = rag.query(q)
        print(f"Q: {result['question']}")
        print(f"Sources: {[r['source'] for r in result['retrieved'][:3]]}")
        print(f"A: {result['answer'][:300]}...")
        print(f"Tokens: {result['tokens_used']} | Model: {result['model']}")


if __name__ == '__main__':
    main()
