// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Training data loader — reads JSONL QA pairs for RAFT fine-tuning.
//!
//! Supports two formats:
//! 1. Full RAFT: question + source_content + distractor_chunks → answer
//!    (model learns to extract answers from context, ignoring distractors)
//! 2. Simple Q/A: question → answer (for distilled pairs without context)

use serde::Deserialize;
use std::path::Path;

/// A distractor chunk from the RAFT dataset.
#[derive(Debug, Deserialize, Clone)]
pub struct DistractorChunk {
    #[serde(default)]
    pub content: String,
    #[serde(default)]
    pub source: String,
    #[serde(default)]
    pub chunk_id: String,
}

/// A single QA training example. Supports both RAFT and simple formats.
#[derive(Debug, Deserialize)]
pub struct TrainingExample {
    pub question: String,
    pub answer: String,
    #[serde(default)]
    pub source: String,
    #[serde(default, alias = "source_file")]
    pub source_file: String,
    #[serde(default)]
    pub source_content: String,
    #[serde(default)]
    pub source_chunk_id: String,
    #[serde(default)]
    pub distractor_chunks: Vec<DistractorChunk>,
    // Legacy field from older datasets
    #[serde(default)]
    pub distractors: Vec<String>,
}

impl TrainingExample {
    /// Returns true if this example has RAFT context (source + distractors).
    pub fn has_raft_context(&self) -> bool {
        !self.source_content.is_empty() && !self.distractor_chunks.is_empty()
    }
}

/// Loaded training dataset with train/eval split.
pub struct Dataset {
    pub train: Vec<TrainingExample>,
    pub eval: Vec<TrainingExample>,
}

/// Maximum chars per document in RAFT prompt (prevents token overflow).
const MAX_DOC_CHARS: usize = 800;

impl Dataset {
    /// Load from a JSONL file, splitting into 90% train / 10% eval.
    pub fn load(path: &str, eval_ratio: f32) -> Result<Self, String> {
        let content = std::fs::read_to_string(path)
            .map_err(|e| format!("Failed to read {}: {}", path, e))?;

        let mut examples: Vec<TrainingExample> = content.lines()
            .filter(|l| !l.trim().is_empty())
            .filter_map(|l| serde_json::from_str(l).ok())
            .collect();

        if examples.is_empty() {
            return Err(format!("No valid examples found in {}", path));
        }

        // Deterministic shuffle using simple swap
        let n = examples.len();
        let mut seed: u64 = 42;
        for i in (1..n).rev() {
            seed = seed.wrapping_mul(6364136223846793005).wrapping_add(1);
            let j = (seed >> 33) as usize % (i + 1);
            examples.swap(i, j);
        }

        let split = (n as f32 * (1.0 - eval_ratio)) as usize;
        let eval = examples.split_off(split);

        Ok(Dataset {
            train: examples,
            eval,
        })
    }

    /// Format a training example as a Mistral instruct prompt.
    ///
    /// Two formats depending on available context:
    ///
    /// RAFT format (source_content + distractors present):
    ///   [INST] system prompt + numbered documents + question [/INST] answer
    ///   Model learns: which document has the answer, how to extract it.
    ///
    /// Simple format (no context):
    ///   [INST] system prompt + question [/INST] answer
    ///   Model learns: direct Q/A from memorized knowledge.
    pub fn format_prompt(example: &TrainingExample) -> String {
        if example.has_raft_context() {
            Self::format_raft_prompt(example)
        } else {
            Self::format_simple_prompt(example)
        }
    }

    /// RAFT format: question + source document + distractor documents → answer.
    /// Document order is shuffled (source not always first) to prevent positional bias.
    fn format_raft_prompt(example: &TrainingExample) -> String {
        // Build document list: source + up to 3 distractors
        let mut docs: Vec<(String, bool)> = Vec::new();

        // Source document (truncated)
        let src = truncate_doc(&example.source_content, MAX_DOC_CHARS);
        docs.push((src, true));

        // Distractor documents (truncated)
        for d in example.distractor_chunks.iter().take(3) {
            if !d.content.is_empty() {
                let content = truncate_doc(&d.content, MAX_DOC_CHARS);
                docs.push((content, false));
            }
        }

        // Shuffle documents (deterministic based on question hash)
        let hash = simple_hash(&example.question);
        let n = docs.len();
        if n > 1 {
            for i in (1..n).rev() {
                let j = (hash.wrapping_mul((i + 1) as u64) >> 32) as usize % (i + 1);
                docs.swap(i, j);
            }
        }

        // Build prompt with numbered documents
        let mut prompt = String::with_capacity(2048);
        prompt.push_str("<s>[INST] You are Zhenai, champion of the Unheaded Kingdom. ");
        prompt.push_str("Answer the question using ONLY the provided documents. ");
        prompt.push_str("If the answer is not in the documents, say so.\n\n");

        for (i, (doc, _is_source)) in docs.iter().enumerate() {
            prompt.push_str(&format!("Document {}:\n{}\n\n", i + 1, doc));
        }

        prompt.push_str(&format!("Question: {} [/INST] {} </s>", example.question, example.answer));
        prompt
    }

    /// Simple format: direct Q/A without context documents.
    fn format_simple_prompt(example: &TrainingExample) -> String {
        format!(
            "<s>[INST] You are Zhenai, champion of the Unheaded Kingdom. \
             Answer using your knowledge of the Unheaded infrastructure platform.\n\n\
             Question: {} [/INST] {} </s>",
            example.question, example.answer
        )
    }

    /// Get dataset statistics.
    pub fn stats(&self) -> DatasetStats {
        let all: Vec<&TrainingExample> = self.train.iter().chain(self.eval.iter()).collect();
        let total_q_chars: usize = all.iter().map(|e| e.question.len()).sum();
        let total_a_chars: usize = all.iter().map(|e| e.answer.len()).sum();
        let raft_count = all.iter().filter(|e| e.has_raft_context()).count();
        let n = all.len();

        DatasetStats {
            total: n,
            train: self.train.len(),
            eval: self.eval.len(),
            avg_q_len: if n > 0 { total_q_chars / n } else { 0 },
            avg_a_len: if n > 0 { total_a_chars / n } else { 0 },
            raft_count,
        }
    }
}

/// Truncate document to max chars, respecting UTF-8 char boundaries.
fn truncate_doc(doc: &str, max_chars: usize) -> String {
    if doc.len() <= max_chars {
        return doc.to_string();
    }
    // Find a valid UTF-8 char boundary at or before max_chars
    let mut end = max_chars;
    while end > 0 && !doc.is_char_boundary(end) {
        end -= 1;
    }
    let truncated = &doc[..end];
    // Find last space to avoid cutting mid-word
    match truncated.rfind(' ') {
        Some(pos) => format!("{}...", &truncated[..pos]),
        None => format!("{}...", truncated),
    }
}

/// Simple hash for deterministic document shuffling.
fn simple_hash(s: &str) -> u64 {
    let mut h: u64 = 5381;
    for b in s.bytes() {
        h = h.wrapping_mul(33).wrapping_add(b as u64);
    }
    h
}

pub struct DatasetStats {
    pub total: usize,
    pub train: usize,
    pub eval: usize,
    pub avg_q_len: usize,
    pub avg_a_len: usize,
    pub raft_count: usize,
}

impl std::fmt::Display for DatasetStats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{} total ({} train, {} eval, {} RAFT) | avg Q: {} chars, avg A: {} chars",
            self.total, self.train, self.eval, self.raft_count, self.avg_q_len, self.avg_a_len)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_load_dataset() {
        let path = "/var/zhen/raft_dataset_combined.jsonl";
        if !Path::new(path).exists() {
            println!("Dataset not found, skipping");
            return;
        }

        let ds = Dataset::load(path, 0.1).expect("Failed to load dataset");
        let stats = ds.stats();
        println!("Dataset: {}", stats);
        assert!(stats.total > 3000, "Expected 3000+ examples, got {}", stats.total);
        assert!(stats.train > stats.eval, "Train should be larger than eval");
    }

    #[test]
    fn test_format_raft_prompt() {
        let ex = TrainingExample {
            question: "What port does Wotan use?".into(),
            answer: "Wotan listens on port 18000 for HTTP and 18001 for gRPC.".into(),
            source: "docs/CLAUDE.md".into(),
            source_file: "docs/CLAUDE.md".into(),
            source_content: "Wotan is the Kingdom message bus. It runs on HTTP port 18000 and gRPC port 18001. All services connect to Wotan on startup.".into(),
            source_chunk_id: "abc123".into(),
            distractor_chunks: vec![
                DistractorChunk { content: "The dashboard runs on port 20000 and provides real-time visualization.".into(), source: "docs/dashboard.md".into(), chunk_id: "d1".into() },
                DistractorChunk { content: "Akira is the health daemon monitoring all services via health checks.".into(), source: "docs/akira.md".into(), chunk_id: "d2".into() },
            ],
            distractors: vec![],
        };
        let prompt = Dataset::format_prompt(&ex);
        assert!(prompt.starts_with("<s>[INST]"));
        assert!(prompt.contains("Document 1:"), "Should have numbered documents");
        assert!(prompt.contains("Document 2:"), "Should have distractor docs");
        assert!(prompt.contains("18000"), "Should contain answer content");
        assert!(prompt.contains("provided documents"), "Should reference documents");
        assert!(prompt.ends_with("</s>"));
        println!("RAFT prompt length: {} chars", prompt.len());
    }

    #[test]
    fn test_format_simple_prompt() {
        let ex = TrainingExample {
            question: "What is BGP?".into(),
            answer: "BGP is the Border Gateway Protocol for inter-AS routing.".into(),
            source: "cs/networking/bgp.md".into(),
            source_file: String::new(),
            source_content: String::new(),
            source_chunk_id: String::new(),
            distractor_chunks: vec![],
            distractors: vec![],
        };
        let prompt = Dataset::format_prompt(&ex);
        assert!(prompt.contains("your knowledge"), "Simple format should reference knowledge");
        assert!(!prompt.contains("Document 1:"), "Simple format should NOT have documents");
        assert!(prompt.contains("BGP"));
    }

    #[test]
    fn test_has_raft_context() {
        let with = TrainingExample {
            question: "Q".into(), answer: "A".into(), source: String::new(),
            source_file: String::new(), source_content: "some content".into(),
            source_chunk_id: String::new(),
            distractor_chunks: vec![DistractorChunk { content: "d".into(), source: String::new(), chunk_id: String::new() }],
            distractors: vec![],
        };
        assert!(with.has_raft_context());

        let without = TrainingExample {
            question: "Q".into(), answer: "A".into(), source: String::new(),
            source_file: String::new(), source_content: String::new(),
            source_chunk_id: String::new(), distractor_chunks: vec![], distractors: vec![],
        };
        assert!(!without.has_raft_context());
    }

    #[test]
    fn test_truncate_doc() {
        assert_eq!(truncate_doc("short", 100), "short");
        let long = "word1 word2 word3 word4 word5 word6";
        let t = truncate_doc(long, 15);
        assert!(t.ends_with("..."));
        assert!(t.len() <= 18); // 15 + "..."
    }

    #[test]
    fn test_raft_dataset_has_context() {
        let path = "/var/zhen/raft_dataset_combined.jsonl";
        if !Path::new(path).exists() {
            println!("Dataset not found, skipping");
            return;
        }
        let ds = Dataset::load(path, 0.1).expect("load");
        let stats = ds.stats();
        println!("RAFT examples: {}/{}", stats.raft_count, stats.total);
        assert!(stats.raft_count > 0, "Should have RAFT-format examples with context");
    }
}
