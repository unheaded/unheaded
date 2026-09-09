// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Quantization/dequantization for GGML tensor types.
//!
//! Supports reading quantized weights from GGUF files and dequantizing
//! to f32 for GPU matrix multiplication. The dequantization happens
//! tile-by-tile to minimize memory footprint.

/// bf16 → f32: bf16 is the upper 16 bits of an f32 (1 sign + 8 exp + 7 mant).
/// Convert by zero-extending the mantissa to 16 bits.
pub fn bf16_to_f32(b: u16) -> f32 {
    f32::from_bits((b as u32) << 16)
}

/// Dequantize a contiguous bf16 tensor to f32. 2 bytes per element, little-endian.
/// Used for Gemma 4 base weights (318 of 601 tensors in E2B-it).
pub fn dequantize_bf16(data: &[u8], n_elements: usize) -> Vec<f32> {
    let mut out = Vec::with_capacity(n_elements);
    let mut i = 0;
    while i + 1 < data.len() && out.len() < n_elements {
        let raw = u16::from_le_bytes([data[i], data[i + 1]]);
        out.push(bf16_to_f32(raw));
        i += 2;
    }
    out
}

/// Q5_K block layout (256 elements per block):
///
/// ```text
/// struct block_q5_K {
///     half d;              // 2 bytes: super-block scale (delta)
///     half dmin;           // 2 bytes: super-block min
///     uint8_t scales[12];  // 12 bytes: sub-block scales and mins
///     uint8_t qh[32];      // 32 bytes: high bits of quants
///     uint8_t qs[128];     // 128 bytes: low 4 bits of quants
/// };
/// ```
///
/// Total: 176 bytes per 256 elements = 5.5 bits/element
pub const Q5_K_BLOCK_SIZE: usize = 256;
pub const Q5_K_BYTES_PER_BLOCK: usize = 176;

/// Dequantize a single Q5_K block (256 elements) to f32.
///
/// This is the CPU reference implementation. GPU kernel will mirror this logic.
pub fn dequantize_q5_k_block(block: &[u8], output: &mut [f32]) {
    assert!(block.len() >= Q5_K_BYTES_PER_BLOCK);
    assert!(output.len() >= Q5_K_BLOCK_SIZE);

    // Parse block header
    let d = f16_to_f32(u16::from_le_bytes([block[0], block[1]]));
    let dmin = f16_to_f32(u16::from_le_bytes([block[2], block[3]]));

    // Scales and mins are packed in block[4..16]
    let scales_raw = &block[4..16];

    // qh = high bits, block[16..48]
    let qh = &block[16..48];

    // qs = low 4 bits, block[48..176]
    let qs = &block[48..176];

    // Dequantize 8 sub-blocks of 32 elements each
    for j in 0..Q5_K_BLOCK_SIZE / 32 {
        // Extract scale and min for this sub-block
        let (sc, m) = get_scale_min_k4(j, scales_raw);
        let d_sc = d * sc as f32;
        let d_m = dmin * m as f32;

        for l in 0..32 {
            let idx = j * 32 + l;
            // Low 4 bits from qs
            let ql = if l < 16 {
                qs[j * 16 + l] & 0x0F
            } else {
                qs[j * 16 + l - 16] >> 4
            };
            // High bit from qh
            let qh_bit = (qh[l] >> j) & 1;
            // 5-bit value
            let q = ql | (qh_bit << 4);
            output[idx] = d_sc * q as f32 - d_m;
        }
    }
}

/// Dequantize a full tensor of Q5_K blocks to f32.
pub fn dequantize_q5_k(data: &[u8], num_elements: usize) -> Vec<f32> {
    let num_blocks = num_elements / Q5_K_BLOCK_SIZE;
    let mut output = vec![0.0f32; num_elements];

    for i in 0..num_blocks {
        let block_start = i * Q5_K_BYTES_PER_BLOCK;
        let block_end = block_start + Q5_K_BYTES_PER_BLOCK;
        if block_end > data.len() {
            break;
        }
        let out_start = i * Q5_K_BLOCK_SIZE;
        dequantize_q5_k_block(
            &data[block_start..block_end],
            &mut output[out_start..out_start + Q5_K_BLOCK_SIZE],
        );
    }

    output
}

/// Q6_K block layout (256 elements per block):
///
/// struct block_q6_K {
///     uint8_t ql[128];     // low 4 bits of quants
///     uint8_t qh[64];      // high 2 bits of quants
///     int8_t  scales[16];  // scales (signed)
///     half    d;           // super-block scale
/// };
///
/// Total: 210 bytes per 256 elements = 6.5625 bits/element
pub const Q6_K_BLOCK_SIZE: usize = 256;
pub const Q6_K_BYTES_PER_BLOCK: usize = 210;

/// Dequantize a single Q6_K block (256 elements) to f32.
pub fn dequantize_q6_k_block(block: &[u8], output: &mut [f32]) {
    assert!(block.len() >= Q6_K_BYTES_PER_BLOCK);
    assert!(output.len() >= Q6_K_BLOCK_SIZE);

    let ql = &block[0..128]; // low 4 bits
    let qh = &block[128..192]; // high 2 bits
    let scales = &block[192..208]; // int8 scales
    let d = f16_to_f32(u16::from_le_bytes([block[208], block[209]]));

    for i in 0..Q6_K_BLOCK_SIZE {
        // Extract 6-bit value
        let ql_val = if i < 128 {
            if i < 64 {
                (ql[i] & 0x0F) as i8
            } else {
                (ql[i - 64] >> 4) as i8
            }
        } else {
            if i < 192 {
                (ql[i - 64] & 0x0F) as i8
            } else {
                (ql[i - 128] >> 4) as i8
            }
        };

        let qh_val = ((qh[i % 64] >> ((i / 64) * 2)) & 3) as i8;
        let q = ql_val | (qh_val << 4); // 6-bit signed value (-32 to 31)

        // Scale
        let scale_idx = i / 16;
        let sc = scales[scale_idx.min(15)] as i8;

        output[i] = d * sc as f32 * q as f32;
    }
}

/// Dequantize a full tensor of Q6_K blocks to f32.
pub fn dequantize_q6_k(data: &[u8], num_elements: usize) -> Vec<f32> {
    let num_blocks = num_elements / Q6_K_BLOCK_SIZE;
    let mut output = vec![0.0f32; num_elements];

    for i in 0..num_blocks {
        let block_start = i * Q6_K_BYTES_PER_BLOCK;
        let block_end = block_start + Q6_K_BYTES_PER_BLOCK;
        if block_end > data.len() {
            break;
        }
        let out_start = i * Q6_K_BLOCK_SIZE;
        dequantize_q6_k_block(
            &data[block_start..block_end],
            &mut output[out_start..out_start + Q6_K_BLOCK_SIZE],
        );
    }

    output
}

/// Extract scale and min for a sub-block from the packed scales array.
/// K-quant scales are packed 6 bits each into 12 bytes.
fn get_scale_min_k4(j: usize, scales: &[u8]) -> (u8, u8) {
    if j < 4 {
        let sc = scales[j] & 63;
        let m = scales[j + 4] & 63;
        (sc, m)
    } else {
        let j2 = j - 4;
        let sc = (scales[j2 + 8] & 0x0F) | ((scales[j2] >> 6) << 4);
        let m = (scales[j2 + 8] >> 4) | ((scales[j2 + 4] >> 6) << 4);
        (sc, m)
    }
}

/// Convert f16 (IEEE 754 half-precision) to f32.
fn f16_to_f32(bits: u16) -> f32 {
    let sign = ((bits >> 15) & 1) as u32;
    let exp = ((bits >> 10) & 0x1F) as u32;
    let frac = (bits & 0x3FF) as u32;

    if exp == 0 {
        if frac == 0 {
            // Zero
            f32::from_bits(sign << 31)
        } else {
            // Subnormal
            let mut e = 0u32;
            let mut f = frac;
            while (f & 0x400) == 0 {
                f <<= 1;
                e += 1;
            }
            f &= 0x3FF;
            let exp32 = 127 - 15 - e;
            f32::from_bits((sign << 31) | (exp32 << 23) | (f << 13))
        }
    } else if exp == 31 {
        // Inf or NaN
        if frac == 0 {
            f32::from_bits((sign << 31) | (0xFF << 23))
        } else {
            f32::from_bits((sign << 31) | (0xFF << 23) | (frac << 13))
        }
    } else {
        // Normal
        let exp32 = exp + 127 - 15;
        f32::from_bits((sign << 31) | (exp32 << 23) | (frac << 13))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_f16_to_f32() {
        // 1.0 in f16 = 0x3C00
        assert!((f16_to_f32(0x3C00) - 1.0).abs() < 1e-6);
        // 0.5 in f16 = 0x3800
        assert!((f16_to_f32(0x3800) - 0.5).abs() < 1e-6);
        // -1.0 in f16 = 0xBC00
        assert!((f16_to_f32(0xBC00) - (-1.0)).abs() < 1e-6);
        // 0.0 in f16 = 0x0000
        assert_eq!(f16_to_f32(0x0000), 0.0);
    }

    #[test]
    fn test_dequantize_q5_k_block_zeros() {
        // All-zero block should produce all-zero output
        let block = vec![0u8; Q5_K_BYTES_PER_BLOCK];
        let mut output = vec![0.0f32; Q5_K_BLOCK_SIZE];
        dequantize_q5_k_block(&block, &mut output);
        // With d=0 and dmin=0, everything should be 0
        for v in &output {
            assert_eq!(*v, 0.0);
        }
    }

    #[test]
    fn test_dequantize_q5_k_nonzero() {
        // Create a block with known d, dmin, and quants
        let mut block = vec![0u8; Q5_K_BYTES_PER_BLOCK];
        // d = 1.0 in f16 (0x3C00 LE = [0x00, 0x3C])
        block[0] = 0x00;
        block[1] = 0x3C;
        // dmin = 0.0
        block[2] = 0x00;
        block[3] = 0x00;
        // scales[0] = 1 (first sub-block scale = 1)
        block[4] = 1;
        // qs: set first value to 5 (0x05)
        block[48] = 0x05;

        let mut output = vec![0.0f32; Q5_K_BLOCK_SIZE];
        dequantize_q5_k_block(&block, &mut output);

        // First element: d * scale * q - dmin * min = 1.0 * 1 * 5 - 0 = 5.0
        assert!(
            (output[0] - 5.0).abs() < 0.01,
            "Expected ~5.0, got {}",
            output[0]
        );
    }

    #[test]
    fn test_q5_k_block_size() {
        assert_eq!(Q5_K_BLOCK_SIZE, 256);
        assert_eq!(Q5_K_BYTES_PER_BLOCK, 176);
        // 176 bytes / 256 elements = 5.5 bits per element
        assert!((Q5_K_BYTES_PER_BLOCK as f64 * 8.0 / Q5_K_BLOCK_SIZE as f64 - 5.5).abs() < 0.01);
    }
}
