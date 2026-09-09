// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! GGUF-native tokenizer — reads vocabulary directly from model file.
//!
//! No external dependencies. Builds token lookup from GGUF metadata.
//! Implements greedy longest-match tokenization (BPE-like).

use std::collections::HashMap;

/// Tokenizer built from GGUF vocabulary.
pub struct Tokenizer {
    /// Token string → token ID
    vocab: HashMap<String, u32>,
    /// Token ID → token string (for decoding)
    id_to_token: Vec<String>,
    /// Special tokens
    pub bos_id: u32,
    pub eos_id: u32,
    pub vocab_size: u32,
}

impl Tokenizer {
    /// Build tokenizer from GGUF token vocabulary.
    /// Tokens are extracted during GGUF parsing as individual strings.
    pub fn from_tokens(tokens: Vec<String>, bos_id: u32, eos_id: u32) -> Self {
        let vocab_size = tokens.len() as u32;
        let mut vocab = HashMap::with_capacity(tokens.len());
        for (i, tok) in tokens.iter().enumerate() {
            vocab.insert(tok.clone(), i as u32);
        }

        Tokenizer {
            vocab,
            id_to_token: tokens,
            bos_id,
            eos_id,
            vocab_size,
        }
    }

    /// Tokenize text using greedy longest-match.
    /// Returns token IDs with BOS prepended.
    pub fn encode(&self, text: &str) -> Vec<u32> {
        let mut ids = vec![self.bos_id]; // Start with BOS

        // Convert text to bytes for byte-level fallback
        let bytes = text.as_bytes();
        let mut pos = 0;

        while pos < bytes.len() {
            let mut best_len = 0;
            let mut best_id = None;

            // Try progressively shorter matches from current position
            // Max token length in sentencepiece is typically ~64 bytes
            let max_len = (bytes.len() - pos).min(64);

            for len in (1..=max_len).rev() {
                let candidate = &bytes[pos..pos + len];
                if let Ok(s) = std::str::from_utf8(candidate) {
                    // Try the raw string
                    if let Some(&id) = self.vocab.get(s) {
                        best_len = len;
                        best_id = Some(id);
                        break;
                    }
                    // Try with sentencepiece space marker (▁ = \xe2\x96\x81)
                    if pos == 0 || bytes[pos - 1] == b' ' {
                        let with_space = format!("▁{}", s);
                        if let Some(&id) = self.vocab.get(&with_space) {
                            best_len = len;
                            best_id = Some(id);
                            break;
                        }
                    }
                }
            }

            if let Some(id) = best_id {
                ids.push(id);
                pos += best_len;
            } else {
                // Byte-level fallback: encode as <0xNN> token
                let byte_tok = format!("<0x{:02X}>", bytes[pos]);
                if let Some(&id) = self.vocab.get(&byte_tok) {
                    ids.push(id);
                } else {
                    // Unknown byte — use UNK token (0)
                    ids.push(0);
                }
                pos += 1;
            }
        }

        ids
    }

    /// Decode token IDs back to text.
    pub fn decode(&self, ids: &[u32]) -> String {
        let mut text = String::new();
        for &id in ids {
            if id == self.bos_id || id == self.eos_id {
                continue;
            }
            if (id as usize) < self.id_to_token.len() {
                let tok = &self.id_to_token[id as usize];
                // Replace sentencepiece space marker with actual space
                text.push_str(&tok.replace('▁', " "));
            }
        }
        text
    }

    /// Get token string for an ID.
    pub fn get_token(&self, id: u32) -> Option<&str> {
        self.id_to_token.get(id as usize).map(|s| s.as_str())
    }
}

/// Extract token vocabulary from GGUF file by re-reading the metadata.
/// This is a separate function because the main GGUF parser stringifies arrays.
pub fn extract_vocabulary_from_gguf(path: &str) -> Result<Vec<String>, String> {
    use memmap2::Mmap;
    use std::fs::File;

    let file = File::open(path).map_err(|e| format!("Open: {}", e))?;
    let mmap = unsafe { Mmap::map(&file) }.map_err(|e| format!("Mmap: {}", e))?;

    let data = &mmap[..];
    let mut pos = 0;

    // Skip magic + version
    pos += 4; // magic
    pos += 4; // version

    // Counts
    let _n_tensors = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap());
    pos += 8;
    let n_metadata = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap());
    pos += 8;

    // Scan metadata for tokenizer.ggml.tokens
    for _ in 0..n_metadata {
        // Read key
        let key_len = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap()) as usize;
        pos += 8;
        let key = std::str::from_utf8(&data[pos..pos + key_len])
            .unwrap_or("")
            .to_string();
        pos += key_len;

        // Read value type
        let vtype = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap());
        pos += 4;

        if key == "tokenizer.ggml.tokens" && vtype == 9 {
            // ARRAY type
            let elem_type = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap());
            pos += 4;
            let count = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap()) as usize;
            pos += 8;

            if elem_type != 8 {
                // Must be STRING array
                return Err(format!("Token array is type {} not STRING", elem_type));
            }

            let mut tokens = Vec::with_capacity(count);
            for _ in 0..count {
                let str_len = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap()) as usize;
                pos += 8;
                let s = std::str::from_utf8(&data[pos..pos + str_len])
                    .unwrap_or("")
                    .to_string();
                pos += str_len;
                tokens.push(s);
            }

            return Ok(tokens);
        } else {
            // Skip this value
            pos = skip_gguf_value(data, pos, vtype);
        }
    }

    Err("tokenizer.ggml.tokens not found in GGUF".into())
}

/// Skip over a GGUF value in the byte stream.
fn skip_gguf_value(data: &[u8], mut pos: usize, vtype: u32) -> usize {
    match vtype {
        0 => pos + 1, // u8
        1 => pos + 1, // i8
        2 => pos + 2, // u16
        3 => pos + 2, // i16
        4 => pos + 4, // u32
        5 => pos + 4, // i32
        6 => pos + 4, // f32
        7 => pos + 1, // bool
        8 => {
            // string
            let len = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap()) as usize;
            pos + 8 + len
        }
        9 => {
            // array
            let elem_type = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap());
            pos += 4;
            let count = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap()) as usize;
            pos += 8;
            for _ in 0..count {
                pos = skip_gguf_value(data, pos, elem_type);
            }
            pos
        }
        10 => pos + 8, // u64
        11 => pos + 8, // i64
        12 => pos + 8, // f64
        _ => pos + 4,  // unknown, guess 4 bytes
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_vocabulary() {
        let path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let tokens = extract_vocabulary_from_gguf(path).expect("Failed to extract vocabulary");
        println!("Vocabulary size: {}", tokens.len());
        assert_eq!(tokens.len(), 32000, "Mistral-7B should have 32000 tokens");

        // Check known tokens
        println!("Token 0: {:?}", tokens[0]); // <unk>
        println!("Token 1: {:?}", tokens[1]); // <s> (BOS)
        println!("Token 2: {:?}", tokens[2]); // </s> (EOS)
        println!("Token 29871: {:?}", tokens.get(29871)); // space in sentencepiece

        assert_eq!(tokens[0], "<unk>");
        assert_eq!(tokens[1], "<s>");
        assert_eq!(tokens[2], "</s>");
    }

    #[test]
    fn test_tokenizer_encode() {
        let path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let tokens = extract_vocabulary_from_gguf(path).expect("vocab");
        let tokenizer = Tokenizer::from_tokens(tokens, 1, 2);

        let ids = tokenizer.encode("Hello");
        println!("'Hello' → {:?}", ids);
        assert!(ids.len() >= 2, "Should have at least BOS + 1 token");
        assert_eq!(ids[0], 1, "First token should be BOS");
        assert!(ids.len() < 20, "Should not produce too many tokens");

        // Decode back
        let decoded = tokenizer.decode(&ids);
        println!("Decoded: {:?}", decoded);
        assert!(
            decoded.contains("Hello") || decoded.contains("ello"),
            "Decoded should contain original text"
        );
    }

    #[test]
    fn test_tokenizer_kingdom_text() {
        let path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let tokens = extract_vocabulary_from_gguf(path).expect("vocab");
        let tokenizer = Tokenizer::from_tokens(tokens, 1, 2);

        let ids = tokenizer.encode("What port does Wotan use for gRPC?");
        println!("Kingdom query: {:?} ({} tokens)", ids, ids.len());
        assert!(ids.len() < 30, "Should tokenize efficiently");
        assert!(ids.len() > 3, "Should produce multiple tokens");
    }
}
