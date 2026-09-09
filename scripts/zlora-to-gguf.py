#!/usr/bin/env python3
"""
zlora-to-gguf.py — Convert Kingdom .zlora format to GGUF LoRA adapter.

Reads a .zlora file (Zhenai Forge output) and converts it to a GGUF LoRA
file that llama-server can load via the --lora flag.

Usage:
  python3 scripts/zlora-to-gguf.py --input kingdom.zlora --output kingdom-lora.gguf --base-model model.gguf
"""

import argparse
import os
import struct


def read_zlora(path):
    """Read a .zlora file and extract LoRA weights."""
    with open(path, 'rb') as f:
        # Magic: "ZLORA\x01" (6 bytes)
        magic = f.read(6)
        if magic != b'ZLORA\x01':
            raise ValueError(f"Not a .zlora file (magic: {magic!r})")

        # Header
        n_layers = struct.unpack('<I', f.read(4))[0]
        n_embd = struct.unpack('<I', f.read(4))[0]
        rank = struct.unpack('<I', f.read(4))[0]
        alpha = struct.unpack('<f', f.read(4))[0]

        print(f"  .zlora: {n_layers} layers, {n_embd} dim, rank {rank}, alpha {alpha}")

        # Read LoRA weights: [layer][target][A then B]
        # A: (input_dim × rank), B: (rank × output_dim)
        a_size = n_embd * rank
        b_size = rank * n_embd
        total_params = 0

        layers = []
        for l in range(n_layers):
            targets = []
            for t in range(4):  # Q, K, V, O
                a = struct.unpack(f'<{a_size}f', f.read(a_size * 4))
                b = struct.unpack(f'<{b_size}f', f.read(b_size * 4))
                targets.append((a, b))
                total_params += a_size + b_size
            layers.append(targets)

        print(f"  Params: {total_params:,} ({total_params * 4 / 1e6:.1f} MB)")
        return {
            'n_layers': n_layers,
            'n_embd': n_embd,
            'rank': rank,
            'alpha': alpha,
            'layers': layers,
        }


def write_gguf_lora(zlora, output_path):
    """Write LoRA weights as a GGUF file compatible with llama-server --lora."""
    # GGUF LoRA format used by llama.cpp:
    # Standard GGUF with tensor names like "blk.0.attn_q.weight.lora_a"

    print(f"\n  Writing GGUF LoRA to: {output_path}")

    target_names = ['attn_q', 'attn_k', 'attn_v', 'attn_output']
    n = zlora['n_embd']
    r = zlora['rank']
    # Mistral-7B uses GQA: 32 heads for Q/O but only 8 heads for K/V
    # head_dim = n_embd / n_head = 4096 / 32 = 128
    # K/V dim = n_head_kv * head_dim = 8 * 128 = 1024
    n_head_kv = 8
    head_dim = n // 32  # 128
    kv_dim = n_head_kv * head_dim  # 1024
    # Output dimension per target: Q=4096, K=1024, V=1024, O=4096
    target_out_dims = [n, kv_dim, kv_dim, n]

    # For now, write a simple binary that can be loaded
    # Full GGUF LoRA format requires proper GGUF header + tensor info
    # This is a placeholder — proper GGUF writer needed for llama-server compat

    with open(output_path, 'wb') as f:
        # GGUF magic
        f.write(struct.pack('<I', 0x46554747))  # "GGUF"
        f.write(struct.pack('<I', 3))  # version 3
        n_tensors = zlora['n_layers'] * 4 * 2  # 4 targets × 2 (A+B) per layer
        f.write(struct.pack('<Q', n_tensors))  # tensor count
        f.write(struct.pack('<Q', 4))  # metadata count

        # Metadata: general.architecture = "llama"
        write_gguf_string(f, "general.architecture")
        f.write(struct.pack('<I', 8))  # STRING type
        write_gguf_string(f, "llama")

        # Metadata: general.type = "adapter"
        write_gguf_string(f, "general.type")
        f.write(struct.pack('<I', 8))
        write_gguf_string(f, "adapter")

        # Metadata: adapter.type = "lora" (required by llama.cpp)
        write_gguf_string(f, "adapter.type")
        f.write(struct.pack('<I', 8))
        write_gguf_string(f, "lora")

        # Metadata: adapter.lora.alpha = float32 (required by llama.cpp)
        write_gguf_string(f, "adapter.lora.alpha")
        f.write(struct.pack('<I', 6))  # FLOAT32 type
        f.write(struct.pack('<f', zlora['alpha']))

        # Tensor info
        # llama.cpp shape validation (from llama-adapter.cpp):
        #   model_tensor->ne[0] == w.a->ne[0]   (lora_a ne[0] must match model ne[0])
        #   model_tensor->ne[1] == w.b->ne[1]   (lora_b ne[1] must match model ne[1])
        #   w.a->ne[1] == w.b->ne[0]            (rank dimension must match)
        #
        # For Mistral-7B model tensor shapes (GGUF ne[0] x ne[1]):
        #   attn_q.weight:      ne[0]=4096, ne[1]=4096
        #   attn_k.weight:      ne[0]=4096, ne[1]=1024  (GQA: 8 heads × 128)
        #   attn_v.weight:      ne[0]=4096, ne[1]=1024
        #   attn_output.weight: ne[0]=4096, ne[1]=4096
        #
        # So lora_a: ne[0]=model_ne0=4096 (always), ne[1]=rank
        #    lora_b: ne[0]=rank,                     ne[1]=model_ne1
        #
        # model_ne1 per target: Q=4096, K=1024, V=1024, O=4096
        model_ne1 = [n, kv_dim, kv_dim, n]

        offset = 0
        tensor_data = []
        for l in range(zlora['n_layers']):
            for t, tname in enumerate(target_names):
                a_raw, b_raw = zlora['layers'][l][t]
                ne1 = model_ne1[t]  # model's ne[1] for this target

                # Forge stored A as (n_embd × rank) row-major — always correct since model ne[0]=4096=n_embd
                a_data = list(a_raw)  # Full (n_embd × rank) = (4096 × 16)

                # Forge stored B as (rank × n_embd) row-major
                # For K/V, model ne[1]=1024, so we need (rank × 1024) — truncate columns
                if ne1 != n:
                    b_data = []
                    for row in range(r):
                        for col in range(ne1):
                            b_data.append(b_raw[row * n + col])
                else:
                    b_data = list(b_raw)

                # LoRA A: ne[0]=4096 (=model ne[0]), ne[1]=rank
                name_a = f"blk.{l}.{tname}.weight.lora_a"
                write_gguf_string(f, name_a)
                f.write(struct.pack('<I', 2))  # 2 dims
                f.write(struct.pack('<Q', n))         # ne[0] = 4096 (model ne[0])
                f.write(struct.pack('<Q', r))         # ne[1] = rank
                f.write(struct.pack('<I', 0))  # F32
                f.write(struct.pack('<Q', offset))
                packed_a = struct.pack(f'<{len(a_data)}f', *a_data)
                tensor_data.append(packed_a)
                offset += len(packed_a)

                # LoRA B: ne[0]=rank, ne[1]=model_ne1
                name_b = f"blk.{l}.{tname}.weight.lora_b"
                write_gguf_string(f, name_b)
                f.write(struct.pack('<I', 2))
                f.write(struct.pack('<Q', r))         # ne[0] = rank
                f.write(struct.pack('<Q', ne1))       # ne[1] = model ne[1] (1024 for K/V, 4096 for Q/O)
                f.write(struct.pack('<I', 0))  # F32
                f.write(struct.pack('<Q', offset))
                packed_b = struct.pack(f'<{len(b_data)}f', *b_data)
                tensor_data.append(packed_b)
                offset += len(packed_b)

        # Align to 32 bytes
        pos = f.tell()
        padding = (32 - (pos % 32)) % 32
        f.write(b'\x00' * padding)

        # Write tensor data
        f.writelines(tensor_data)

    size_mb = os.path.getsize(output_path) / 1e6
    print(f"  Written: {output_path} ({size_mb:.1f} MB)")
    print(f"  Load with: llama-server --lora {output_path}")


def write_gguf_string(f, s):
    """Write a GGUF string (u64 length + bytes)."""
    encoded = s.encode('utf-8')
    f.write(struct.pack('<Q', len(encoded)))
    f.write(encoded)


def main():
    parser = argparse.ArgumentParser(description='Convert .zlora to GGUF LoRA')
    parser.add_argument('--input', '-i', required=True, help='.zlora file')
    parser.add_argument('--output', '-o', required=True, help='Output GGUF LoRA file')
    args = parser.parse_args()

    print("=== ZLORA → GGUF Converter ===")
    zlora = read_zlora(args.input)
    write_gguf_lora(zlora, args.output)
    print("\nDone.")


if __name__ == '__main__':
    main()
