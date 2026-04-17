// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! GGUF file reader — zero-copy mmap-based parsing.
//! Reads quantized model files without decompressing to fp16.

use memmap2::Mmap;
use std::collections::HashMap;
use std::fs::File;
use std::io;

const GGUF_MAGIC: u32 = 0x46554747; // "GGUF" as LE u32: bytes [0x47,0x47,0x55,0x46]

/// Parsed GGUF file with metadata and tensor descriptors.
pub struct GgufFile {
    pub version: u32,
    pub metadata: HashMap<String, String>,
    pub tensors: Vec<TensorInfo>,
    pub file_size: u64,
    /// Offset in the mmap where tensor data begins (after headers)
    pub data_offset: usize,
    _mmap: Mmap,
}

impl GgufFile {
    /// Get raw bytes for a tensor from the mmap'd file.
    /// Zero-copy — returns a slice into the mmap.
    pub fn tensor_data(&self, tensor: &TensorInfo) -> &[u8] {
        let start = self.data_offset + tensor.offset as usize;
        let end = start + tensor.byte_size as usize;
        &self._mmap[start..end.min(self._mmap.len())]
    }
}

/// Descriptor for a tensor in the GGUF file.
pub struct TensorInfo {
    pub name: String,
    pub tensor_type: String,
    pub dimensions: Vec<u64>,
    pub num_elements: u64,
    pub byte_size: u64,
    pub offset: u64,
}

// GGUF value types
const GGUF_TYPE_UINT8: u32 = 0;
const GGUF_TYPE_INT8: u32 = 1;
const GGUF_TYPE_UINT16: u32 = 2;
const GGUF_TYPE_INT16: u32 = 3;
const GGUF_TYPE_UINT32: u32 = 4;
const GGUF_TYPE_INT32: u32 = 5;
const GGUF_TYPE_FLOAT32: u32 = 6;
const GGUF_TYPE_BOOL: u32 = 7;
const GGUF_TYPE_STRING: u32 = 8;
const GGUF_TYPE_ARRAY: u32 = 9;
const GGUF_TYPE_UINT64: u32 = 10;
const GGUF_TYPE_INT64: u32 = 11;
const GGUF_TYPE_FLOAT64: u32 = 12;

// GGUF tensor types (quantization formats). Order MUST match GGML enum.
const GGML_TYPE_NAMES: &[&str] = &[
    "F32", "F16", "Q4_0", "Q4_1", "Q4_2", "Q4_3", "Q5_0", "Q5_1",
    "Q8_0", "Q8_1", "Q2_K", "Q3_K", "Q4_K", "Q5_K", "Q6_K", "Q8_K",
    "IQ2_XXS", "IQ2_XS", "IQ3_XXS", "IQ1_S", "IQ4_NL", "IQ3_S",
    "IQ2_S", "IQ4_XS", "I8", "I16", "I32", "I64", "F64", "IQ1_M",
    "BF16",   // index 30 — Gemma 4 weights ship as bf16
    "Q4_0_4_4", "Q4_0_4_8", "Q4_0_8_8",  // 31, 32, 33 (deprecated tile formats)
    "TQ1_0", "TQ2_0",  // 34, 35 (ternary)
    "IQ4_NL_4_4",      // 36
];

/// Reader cursor over mmap'd bytes.
struct Reader<'a> {
    data: &'a [u8],
    pos: usize,
}

impl<'a> Reader<'a> {
    fn new(data: &'a [u8]) -> Self {
        Self { data, pos: 0 }
    }

    fn read_u8(&mut self) -> u8 {
        let v = self.data[self.pos];
        self.pos += 1;
        v
    }

    fn read_u32(&mut self) -> u32 {
        let v = u32::from_le_bytes(self.data[self.pos..self.pos + 4].try_into().unwrap());
        self.pos += 4;
        v
    }

    fn read_u64(&mut self) -> u64 {
        let v = u64::from_le_bytes(self.data[self.pos..self.pos + 8].try_into().unwrap());
        self.pos += 8;
        v
    }

    fn read_i32(&mut self) -> i32 {
        let v = i32::from_le_bytes(self.data[self.pos..self.pos + 4].try_into().unwrap());
        self.pos += 4;
        v
    }

    fn read_i64(&mut self) -> i64 {
        let v = i64::from_le_bytes(self.data[self.pos..self.pos + 8].try_into().unwrap());
        self.pos += 8;
        v
    }

    fn read_f32(&mut self) -> f32 {
        let v = f32::from_le_bytes(self.data[self.pos..self.pos + 4].try_into().unwrap());
        self.pos += 4;
        v
    }

    fn read_f64(&mut self) -> f64 {
        let v = f64::from_le_bytes(self.data[self.pos..self.pos + 8].try_into().unwrap());
        self.pos += 8;
        v
    }

    fn read_string(&mut self) -> String {
        let len = self.read_u64() as usize;
        let s = std::str::from_utf8(&self.data[self.pos..self.pos + len])
            .unwrap_or("<invalid utf8>")
            .to_string();
        self.pos += len;
        s
    }

    fn read_bool(&mut self) -> bool {
        self.read_u8() != 0
    }

    /// Read a GGUF typed value and return as string representation.
    fn read_value(&mut self, vtype: u32) -> String {
        match vtype {
            GGUF_TYPE_UINT8 => self.read_u8().to_string(),
            GGUF_TYPE_INT8 => (self.read_u8() as i8).to_string(),
            GGUF_TYPE_UINT16 => {
                let v = u16::from_le_bytes(self.data[self.pos..self.pos + 2].try_into().unwrap());
                self.pos += 2;
                v.to_string()
            }
            GGUF_TYPE_INT16 => {
                let v = i16::from_le_bytes(self.data[self.pos..self.pos + 2].try_into().unwrap());
                self.pos += 2;
                v.to_string()
            }
            GGUF_TYPE_UINT32 => self.read_u32().to_string(),
            GGUF_TYPE_INT32 => self.read_i32().to_string(),
            GGUF_TYPE_FLOAT32 => format!("{:.6}", self.read_f32()),
            GGUF_TYPE_BOOL => self.read_bool().to_string(),
            GGUF_TYPE_STRING => self.read_string(),
            GGUF_TYPE_UINT64 => self.read_u64().to_string(),
            GGUF_TYPE_INT64 => self.read_i64().to_string(),
            GGUF_TYPE_FLOAT64 => format!("{:.6}", self.read_f64()),
            GGUF_TYPE_ARRAY => {
                let elem_type = self.read_u32();
                let count = self.read_u64() as usize;
                // For numeric arrays up to 1024 (covers per-layer hparams in 60-layer
                // models), store all values comma-joined so consumers can recover
                // them via get_arch_u32_array. For larger / string arrays, keep the
                // summary format (tokenizer vocabs etc go through different paths).
                let is_numeric = matches!(
                    elem_type,
                    GGUF_TYPE_UINT8
                        | GGUF_TYPE_INT8
                        | GGUF_TYPE_UINT16
                        | GGUF_TYPE_INT16
                        | GGUF_TYPE_UINT32
                        | GGUF_TYPE_INT32
                        | GGUF_TYPE_UINT64
                        | GGUF_TYPE_INT64
                        | GGUF_TYPE_FLOAT32
                        | GGUF_TYPE_FLOAT64
                        | GGUF_TYPE_BOOL
                );
                if is_numeric && count <= 1024 {
                    let vals: Vec<String> =
                        (0..count).map(|_| self.read_value(elem_type)).collect();
                    // Machine-readable comma-separated form with an 'A:' prefix
                    // so callers can detect and split. Primitive accessors
                    // (get_u32/f32/str) still fail cleanly on this marker.
                    format!("A:{}", vals.join(","))
                } else {
                    for _ in 0..count {
                        self.read_value(elem_type);
                    }
                    format!("[array of {} {}s]", count, type_name(elem_type))
                }
            }
            _ => {
                format!("<unknown type {}>", vtype)
            }
        }
    }
}

fn type_name(t: u32) -> &'static str {
    match t {
        GGUF_TYPE_UINT8 => "u8",
        GGUF_TYPE_INT8 => "i8",
        GGUF_TYPE_UINT16 => "u16",
        GGUF_TYPE_INT16 => "i16",
        GGUF_TYPE_UINT32 => "u32",
        GGUF_TYPE_INT32 => "i32",
        GGUF_TYPE_FLOAT32 => "f32",
        GGUF_TYPE_BOOL => "bool",
        GGUF_TYPE_STRING => "string",
        GGUF_TYPE_ARRAY => "array",
        GGUF_TYPE_UINT64 => "u64",
        GGUF_TYPE_INT64 => "i64",
        GGUF_TYPE_FLOAT64 => "f64",
        _ => "unknown",
    }
}

/// Byte size per element for each GGML tensor type.
fn ggml_type_size(ttype: u32) -> u64 {
    match ttype {
        0 => 4,    // F32
        1 => 2,    // F16
        2 => 18,   // Q4_0 (block of 32 elements = 18 bytes)
        3 => 20,   // Q4_1
        6 => 18,   // Q5_0
        7 => 22,   // Q5_1
        8 => 34,   // Q8_0
        9 => 40,   // Q8_1
        10 => 0,   // Q2_K (variable)
        11 => 0,   // Q3_K
        12 => 0,   // Q4_K
        13 => 0,   // Q5_K
        14 => 0,   // Q6_K
        _ => 0,
    }
}

/// Block size for quantized types (how many elements per block).
fn ggml_block_size(ttype: u32) -> u64 {
    match ttype {
        0 | 1 => 1,     // F32, F16 — 1 element per "block"
        2..=9 => 32,    // Q4/Q5/Q8 — 32 elements per block
        10..=14 => 256, // K-quants — 256 elements per block
        _ => 1,
    }
}

impl GgufFile {
    /// Open and parse a GGUF file via mmap (zero-copy).
    pub fn open(path: &str) -> io::Result<Self> {
        let file = File::open(path)?;
        let file_size = file.metadata()?.len();
        let mmap = unsafe { Mmap::map(&file)? };

        let mut reader = Reader::new(&mmap);

        // Magic
        let magic = reader.read_u32();
        if magic != GGUF_MAGIC {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                format!("Not a GGUF file (magic: 0x{:08X}, expected 0x{:08X})", magic, GGUF_MAGIC),
            ));
        }

        // Version
        let version = reader.read_u32();
        if version < 2 || version > 3 {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                format!("Unsupported GGUF version: {} (expected 2 or 3)", version),
            ));
        }

        // Counts
        let n_tensors = reader.read_u64();
        let n_metadata = reader.read_u64();

        // Parse metadata
        let mut metadata = HashMap::new();
        for _ in 0..n_metadata {
            let key = reader.read_string();
            let vtype = reader.read_u32();
            let value = reader.read_value(vtype);
            metadata.insert(key, value);
        }

        // Parse tensor info
        let mut tensors = Vec::with_capacity(n_tensors as usize);
        for _ in 0..n_tensors {
            let name = reader.read_string();
            let n_dims = reader.read_u32();
            let dimensions: Vec<u64> = (0..n_dims).map(|_| reader.read_u64()).collect();
            let ttype = reader.read_u32();
            let offset = reader.read_u64();

            let num_elements = if dimensions.is_empty() { 0 } else { dimensions.iter().product() };

            // Calculate byte size from type and element count
            let block_sz = ggml_block_size(ttype);
            let type_sz = ggml_type_size(ttype);
            let byte_size = if block_sz > 0 && type_sz > 0 {
                (num_elements / block_sz) * type_sz
            } else {
                // For K-quant types, estimate from file
                num_elements * 2 / 3 // rough estimate
            };

            let tensor_type = if (ttype as usize) < GGML_TYPE_NAMES.len() {
                GGML_TYPE_NAMES[ttype as usize].to_string()
            } else {
                format!("type_{}", ttype)
            };

            tensors.push(TensorInfo {
                name,
                tensor_type,
                dimensions,
                num_elements,
                byte_size,
                offset,
            });
        }

        // Data section starts after all headers, aligned to 32 bytes
        let data_offset = (reader.pos + 31) & !31; // Align to 32-byte boundary

        Ok(GgufFile {
            version,
            metadata,
            tensors,
            file_size,
            data_offset,
            _mmap: mmap,
        })
    }

    /// Get a u32 metadata value by key.
    pub fn get_u32(&self, key: &str) -> Option<u32> {
        self.metadata.get(key).and_then(|v| v.parse().ok())
    }

    /// Get a string metadata value by key.
    pub fn get_str(&self, key: &str) -> Option<&str> {
        self.metadata.get(key).map(|s| s.as_str())
    }

    /// Get a f32 metadata value by key.
    pub fn get_f32(&self, key: &str) -> Option<f32> {
        self.metadata.get(key).and_then(|v| v.parse().ok())
    }

    // === WAVE10F Phase 2 — architecture-aware metadata access ===

    /// Detected model architecture from `general.architecture`.
    pub fn architecture(&self) -> Architecture {
        match self.get_str("general.architecture") {
            Some("llama") => Architecture::Llama,
            Some("gemma4") => Architecture::Gemma4,
            Some(other) => Architecture::Other(other.to_string()),
            None => Architecture::Other(String::new()),
        }
    }

    /// Architecture-aware u32 metadata getter — looks up `"{arch}.{key}"`.
    /// E.g., `get_arch_u32("block_count")` → reads `llama.block_count` or `gemma4.block_count`
    /// depending on detected architecture.
    pub fn get_arch_u32(&self, key: &str) -> Option<u32> {
        let arch = self.architecture();
        self.get_u32(&format!("{}.{}", arch.prefix(), key))
    }

    /// Architecture-aware f32 metadata getter.
    pub fn get_arch_f32(&self, key: &str) -> Option<f32> {
        let arch = self.architecture();
        self.get_f32(&format!("{}.{}", arch.prefix(), key))
    }

    /// Architecture-aware string metadata getter. Returns owned String to
    /// avoid lifetime issues from the formatted lookup key.
    pub fn get_arch_string(&self, key: &str) -> Option<String> {
        let prefix = self.architecture().prefix().to_string();
        self.metadata.get(&format!("{}.{}", prefix, key)).cloned()
    }

    /// Get a numeric array metadata value by full key. Returns Vec<u32> if the
    /// stored value is the `A:v1,v2,...` machine-readable array marker.
    pub fn get_u32_array(&self, key: &str) -> Option<Vec<u32>> {
        let v = self.metadata.get(key)?;
        let s = v.strip_prefix("A:")?;
        s.split(',').map(|x| x.trim().parse::<u32>().ok()).collect()
    }

    /// Get a f32 array metadata value by full key.
    pub fn get_f32_array(&self, key: &str) -> Option<Vec<f32>> {
        let v = self.metadata.get(key)?;
        let s = v.strip_prefix("A:")?;
        s.split(',').map(|x| x.trim().parse::<f32>().ok()).collect()
    }

    /// Architecture-aware u32 array getter.
    pub fn get_arch_u32_array(&self, key: &str) -> Option<Vec<u32>> {
        let arch = self.architecture();
        self.get_u32_array(&format!("{}.{}", arch.prefix(), key))
    }

    /// Architecture-aware f32 array getter.
    pub fn get_arch_f32_array(&self, key: &str) -> Option<Vec<f32>> {
        let arch = self.architecture();
        self.get_f32_array(&format!("{}.{}", arch.prefix(), key))
    }

    /// Get a metadata value that may be EITHER a scalar u32 OR the first element
    /// of a u32 array (useful when a key is stored as a per-layer array but we
    /// want the "canonical" value assuming all layers agree).
    pub fn get_arch_u32_or_first(&self, key: &str) -> Option<u32> {
        if let Some(v) = self.get_arch_u32(key) {
            return Some(v);
        }
        self.get_arch_u32_array(key)
            .and_then(|v| v.first().copied())
    }

    /// Same as above for f32.
    pub fn get_arch_f32_or_first(&self, key: &str) -> Option<f32> {
        if let Some(v) = self.get_arch_f32(key) {
            return Some(v);
        }
        self.get_arch_f32_array(key)
            .and_then(|v| v.first().copied())
    }
}

/// Detected model architecture string from GGUF metadata.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Architecture {
    /// Llama family (Mistral, Llama 1/2/3, CodeLlama, etc.) — `general.architecture = "llama"`
    Llama,
    /// Gemma 4 (E2B, E4B, 26B-A4B, 31B) — `general.architecture = "gemma4"`
    Gemma4,
    /// Anything else — keep the literal string for diagnostic purposes.
    Other(String),
}

impl Architecture {
    /// Metadata key prefix used by this architecture. Per llama.cpp convention,
    /// hparams are stored as `"{prefix}.{key}"` (e.g. `llama.block_count`,
    /// `gemma4.embedding_length`).
    pub fn prefix(&self) -> &str {
        match self {
            Architecture::Llama => "llama",
            Architecture::Gemma4 => "gemma4",
            Architecture::Other(s) => s.as_str(),
        }
    }

    /// Human-readable architecture name for display.
    pub fn display_name(&self) -> &str {
        match self {
            Architecture::Llama => "Llama family (Mistral/Llama/CodeLlama)",
            Architecture::Gemma4 => "Gemma 4 (E2B/E4B/26B-A4B/31B)",
            Architecture::Other(s) => s.as_str(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gguf_magic() {
        assert_eq!(GGUF_MAGIC, 0x46554747);
        // "GGUF" in ASCII LE: G=0x47, G=0x47, U=0x55, F=0x46
    }

    #[test]
    fn test_type_sizes() {
        assert_eq!(ggml_type_size(0), 4); // F32
        assert_eq!(ggml_type_size(1), 2); // F16
        assert_eq!(ggml_block_size(0), 1);
        assert_eq!(ggml_block_size(2), 32); // Q4_0
    }
}
