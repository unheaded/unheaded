// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Training data loader — reads JSONL QA pairs for RAFT fine-tuning.

use serde::Deserialize;
use std::path::Path;

/// A single QA training example from the RAFT dataset.
#[derive(Debug, Deserialize)]
pub struct TrainingExample {
    pub question: String,
    pub answer: String,
    #[serde(default)]
    pub source: String,
    #[serde(default)]
    pub distractors: Vec<String>,
}

/// Loaded training dataset with train/eval split.
pub struct Dataset {
    pub train: Vec<TrainingExample>,
    pub eval: Vec<TrainingExample>,
}

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
    pub fn format_prompt(example: &TrainingExample) -> String {
        format!(
            "<s>[INST] You are Zhenai, champion of the Unheaded Kingdom. \
             Answer using the context provided.\n\n\
             Question: {} [/INST] {} </s>",
            example.question, example.answer
        )
    }

    /// Get dataset statistics.
    pub fn stats(&self) -> DatasetStats {
        let all: Vec<&TrainingExample> = self.train.iter().chain(self.eval.iter()).collect();
        let total_q_chars: usize = all.iter().map(|e| e.question.len()).sum();
        let total_a_chars: usize = all.iter().map(|e| e.answer.len()).sum();
        let n = all.len();

        DatasetStats {
            total: n,
            train: self.train.len(),
            eval: self.eval.len(),
            avg_q_len: if n > 0 { total_q_chars / n } else { 0 },
            avg_a_len: if n > 0 { total_a_chars / n } else { 0 },
        }
    }
}

pub struct DatasetStats {
    pub total: usize,
    pub train: usize,
    pub eval: usize,
    pub avg_q_len: usize,
    pub avg_a_len: usize,
}

impl std::fmt::Display for DatasetStats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{} total ({} train, {} eval) | avg Q: {} chars, avg A: {} chars",
            self.total, self.train, self.eval, self.avg_q_len, self.avg_a_len)
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
    fn test_format_prompt() {
        let ex = TrainingExample {
            question: "What port does Wotan use?".into(),
            answer: "Wotan listens on port 18000 for HTTP and 18001 for gRPC.".into(),
            source: "docs/CLAUDE.md".into(),
            distractors: vec![],
        };
        let prompt = Dataset::format_prompt(&ex);
        assert!(prompt.starts_with("<s>[INST]"));
        assert!(prompt.contains("Zhenai"));
        assert!(prompt.contains("18000"));
        assert!(prompt.ends_with("</s>"));
    }
}
