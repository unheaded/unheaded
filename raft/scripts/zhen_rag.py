#!/usr/bin/env python3
"""
Zhen RAG Pipeline — Retrieval-Augmented Generation for Unheaded
Combines FAISS retrieval with Mistral-7B inference via llama.cpp
"""
import json
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

        # Load embedding model
        print("Loading embedding model...")
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')

        # Load FAISS index
        print("Loading FAISS index...")
        self.index = faiss.read_index(str(self.index_dir / 'ring1.index'))

        # Load ID map (can be list or dict depending on how it was saved)
        with open(self.index_dir / 'ring1_ids.json', 'r') as f:
            raw_ids = json.load(f)
            if isinstance(raw_ids, list):
                self.id_map = {str(i): v for i, v in enumerate(raw_ids)}
            else:
                self.id_map = raw_ids

        # Load corpus for content retrieval
        print("Loading corpus...")
        self.corpus = {}
        with open(self.corpus_file, 'r') as f:
            for line in f:
                chunk = json.loads(line)
                self.corpus[chunk['id']] = {
                    'content': chunk['content'],
                    'source': chunk.get('source', ''),
                    'type': chunk.get('type', 'unknown'),
                }

        print(f"RAG Pipeline ready: {len(self.id_map)} vectors, {len(self.corpus)} chunks")

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
            if chunk_id and chunk_id in self.corpus:
                data = self.corpus[chunk_id]
                retrieved.append({
                    'id': chunk_id,
                    'content': data['content'],
                    'source': data['source'],
                    'type': data['type'],
                    'distance': float(distance),
                })
        return retrieved

    def generate(self, query, context_chunks):
        """Generate response using Mistral with retrieved context"""
        context = "\n\n---\n\n".join([
            f"[Source: {c['source']}]\n{c['content'][:1500]}"
            for c in context_chunks[:3]
        ])

        prompt = f"""<s>[INST] You are Zhen, the AI champion of the Unheaded infrastructure platform.
You are an expert on Unheaded's architecture, services, protocols, and codebase.
Use the following context from the Unheaded codebase to answer accurately.
If the context doesn't contain the answer, say so honestly.

CONTEXT:
{context}

QUESTION: {query} [/INST]"""

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
                }
            else:
                return {'answer': f"Inference error: {response.status_code}", 'tokens_used': 0}

        except requests.exceptions.ConnectionError:
            return {'answer': "Error: Inference server not reachable on port 20100", 'tokens_used': 0}
        except requests.exceptions.Timeout:
            return {'answer': "Error: Inference timed out (60s)", 'tokens_used': 0}

    def query(self, question):
        """Full RAG query: retrieve + generate"""
        retrieved = self.retrieve(question, k=5)
        result = self.generate(question, retrieved)
        return {
            'question': question,
            'retrieved': retrieved,
            'answer': result['answer'],
            'tokens_used': result['tokens_used'],
        }


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
        print(f"Tokens: {result['tokens_used']}")


if __name__ == '__main__':
    main()
