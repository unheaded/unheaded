#!/usr/bin/env python3
"""
08_train_qlora.py - QLoRA fine-tuning of Mistral-7B on RAFT training data.

Fine-tunes Mistral-7B-Instruct-v0.2 using 4-bit QLoRA on the RAFT training
dataset prepared by 07_prepare_training.py.

Hardware target: AMD RX 7700 XT (12GB VRAM) via ROCm 6.1.3
Fallback: Second AMD GPU (16GB VRAM)

Usage:
    source ~/.venv/zhen/bin/activate
    pip install peft bitsandbytes trl datasets accelerate transformers
    python 08_train_qlora.py

If unsloth is available, it will be preferred for faster training.
"""

import json
import sys
from pathlib import Path

# Paths
SCRIPT_DIR = Path(__file__).resolve().parent
RAFT_DIR = SCRIPT_DIR.parent
TRAINING_DIR = RAFT_DIR / "training"
TRAIN_PATH = TRAINING_DIR / "train.jsonl"
EVAL_PATH = TRAINING_DIR / "eval.jsonl"
CHECKPOINT_DIR = TRAINING_DIR / "checkpoints"
LOG_DIR = TRAINING_DIR / "logs"

# ============================================================================
# Training Configuration (conservative for 12GB VRAM)
# ============================================================================
CONFIG = {
    "base_model": "mistralai/Mistral-7B-Instruct-v0.2",
    "max_seq_length": 2048,

    # LoRA hyperparameters
    "lora_rank": 16,
    "lora_alpha": 32,
    "lora_dropout": 0.05,
    "target_modules": ["q_proj", "k_proj", "v_proj", "o_proj"],

    # Training hyperparameters
    "learning_rate": 2e-4,
    "per_device_train_batch_size": 1,       # Reduced from 2 — 14GB RAM constraint
    "gradient_accumulation_steps": 8,       # Increased to compensate (effective batch = 8)
    "num_train_epochs": 3,
    "warmup_ratio": 0.03,
    "weight_decay": 0.01,
    "bf16": True,
    "fp16": False,
    "logging_steps": 10,
    "save_steps": 50,
    "eval_steps": 50,
    "save_total_limit": 3,
    "optim": "paged_adamw_8bit",
    "lr_scheduler_type": "cosine",
    "seed": 42,
    "gradient_checkpointing": True,  # saves VRAM at cost of speed

    # Output
    "output_dir": str(CHECKPOINT_DIR),
    "logging_dir": str(LOG_DIR),
}


def check_dependencies():
    """Check that required packages are installed."""
    missing = []
    required = {
        "torch": "torch (with ROCm support)",
        "transformers": "transformers",
        "peft": "peft",
        "trl": "trl",
        "datasets": "datasets",
        "accelerate": "accelerate",
    }

    # Optional but preferred
    optional = {
        "bitsandbytes": "bitsandbytes",
    }

    for module, name in required.items():
        try:
            __import__(module)
        except ImportError:
            missing.append(name)

    has_bnb = True
    for module, name in optional.items():
        try:
            __import__(module)
        except ImportError:
            has_bnb = False
            print(f"WARNING: {name} not installed (needed for 4-bit quantization)")

    if missing:
        print("ERROR: Missing required packages:")
        for m in missing:
            print(f"  - {m}")
        print()
        print("Install with:")
        print("  pip install peft bitsandbytes trl datasets accelerate transformers")
        print()
        print("For ROCm (AMD GPU), you may need:")
        print("  pip install torch --index-url https://download.pytorch.org/whl/rocm6.1")
        print("  pip install bitsandbytes-rocm  # or build from source")
        print()
        sys.exit(1)

    return has_bnb


def check_gpu():
    """Check GPU availability and VRAM."""
    import torch

    if not torch.cuda.is_available():
        # ROCm exposes AMD GPUs through the CUDA API via HIP
        print("WARNING: No GPU detected via torch.cuda")
        print("  If using ROCm, ensure ROCm 6.1.3 is installed and PyTorch ROCm build is used.")
        print("  pip install torch --index-url https://download.pytorch.org/whl/rocm6.1")
        print()
        print("Continuing anyway (will be very slow on CPU)...")
        return

    num_gpus = torch.cuda.device_count()
    print(f"Found {num_gpus} GPU(s):")
    for i in range(num_gpus):
        name = torch.cuda.get_device_name(i)
        mem_total = torch.cuda.get_device_properties(i).total_memory / (1024 ** 3)
        print(f"  GPU {i}: {name} ({mem_total:.1f} GB)")

    # Use first GPU by default (12GB RX 7700 XT)
    primary_mem = torch.cuda.get_device_properties(0).total_memory / (1024 ** 3)
    if primary_mem < 10:
        print(f"WARNING: Primary GPU has only {primary_mem:.1f} GB VRAM.")
        print("  Training may OOM. Consider reducing batch_size to 1.")


def load_training_data():
    """Load train and eval JSONL files into HuggingFace Dataset format."""
    from datasets import Dataset

    def load_jsonl(path):
        examples = []
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line:
                    examples.append(json.loads(line))
        return examples

    train_data = load_jsonl(TRAIN_PATH)
    eval_data = load_jsonl(EVAL_PATH)

    train_dataset = Dataset.from_list(train_data)
    eval_dataset = Dataset.from_list(eval_data)

    print(f"Training examples: {len(train_dataset)}")
    print(f"Evaluation examples: {len(eval_dataset)}")

    return train_dataset, eval_dataset


def setup_model_and_tokenizer(has_bnb: bool):
    """Load base model with 4-bit quantization and apply LoRA."""
    import torch
    from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
    from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig

    print(f"\nLoading base model: {CONFIG['base_model']}")

    # 4-bit quantization config
    if has_bnb:
        bnb_config = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_quant_type="nf4",
            bnb_4bit_compute_dtype=torch.bfloat16,
            bnb_4bit_use_double_quant=True,
        )
    else:
        print("WARNING: bitsandbytes not available. Loading model without quantization.")
        print("  This will use significantly more VRAM and may OOM on 12GB.")
        bnb_config = None

    # Load tokenizer
    tokenizer = AutoTokenizer.from_pretrained(
        CONFIG["base_model"],
        trust_remote_code=True,
    )
    tokenizer.pad_token = tokenizer.eos_token
    tokenizer.padding_side = "right"

    # Load model — use low_cpu_mem_usage to avoid doubling RAM during load
    model_kwargs = {
        "trust_remote_code": True,
        "dtype": torch.bfloat16,
        "device_map": "auto",
        "low_cpu_mem_usage": True,  # Critical for 14GB RAM — loads weights shard-by-shard
    }
    if bnb_config:
        model_kwargs["quantization_config"] = bnb_config

    model = AutoModelForCausalLM.from_pretrained(
        CONFIG["base_model"],
        **model_kwargs,
    )

    if has_bnb:
        model = prepare_model_for_kbit_training(model)

    # LoRA configuration
    lora_config = LoraConfig(
        r=CONFIG["lora_rank"],
        lora_alpha=CONFIG["lora_alpha"],
        lora_dropout=CONFIG["lora_dropout"],
        target_modules=CONFIG["target_modules"],
        bias="none",
        task_type="CAUSAL_LM",
    )

    model = get_peft_model(model, lora_config)

    # Print trainable parameters
    trainable, total = model.get_nb_trainable_parameters()
    print(f"Trainable parameters: {trainable:,} / {total:,} ({100 * trainable / total:.2f}%)")

    return model, tokenizer


def train(model, tokenizer, train_dataset, eval_dataset):
    """Run the QLoRA training loop."""
    from transformers import TrainingArguments
    from trl import SFTTrainer

    # Ensure output directories exist
    CHECKPOINT_DIR.mkdir(parents=True, exist_ok=True)
    LOG_DIR.mkdir(parents=True, exist_ok=True)

    training_args = TrainingArguments(
        output_dir=CONFIG["output_dir"],
        logging_dir=CONFIG["logging_dir"],
        num_train_epochs=CONFIG["num_train_epochs"],
        per_device_train_batch_size=CONFIG["per_device_train_batch_size"],
        per_device_eval_batch_size=CONFIG["per_device_train_batch_size"],
        gradient_accumulation_steps=CONFIG["gradient_accumulation_steps"],
        learning_rate=CONFIG["learning_rate"],
        weight_decay=CONFIG["weight_decay"],
        warmup_ratio=CONFIG["warmup_ratio"],
        lr_scheduler_type=CONFIG["lr_scheduler_type"],
        optim=CONFIG["optim"],
        bf16=CONFIG["bf16"],
        fp16=CONFIG["fp16"],
        logging_steps=CONFIG["logging_steps"],
        save_steps=CONFIG["save_steps"],
        eval_steps=CONFIG["eval_steps"],
        eval_strategy="steps",
        save_total_limit=CONFIG["save_total_limit"],
        gradient_checkpointing=CONFIG["gradient_checkpointing"],
        report_to="none",  # or "tensorboard" if installed
        seed=CONFIG["seed"],
        load_best_model_at_end=True,
        metric_for_best_model="eval_loss",
        greater_is_better=False,
    )

    trainer = SFTTrainer(
        model=model,
        tokenizer=tokenizer,
        train_dataset=train_dataset,
        eval_dataset=eval_dataset,
        args=training_args,
        max_seq_length=CONFIG["max_seq_length"],
        dataset_text_field="text",
        packing=False,  # No packing for instruction-tuning
    )

    print("\nStarting training...")
    print(f"  Epochs: {CONFIG['num_train_epochs']}")
    print(f"  Batch size: {CONFIG['per_device_train_batch_size']}")
    print(f"  Gradient accumulation: {CONFIG['gradient_accumulation_steps']}")
    print(f"  Effective batch size: {CONFIG['per_device_train_batch_size'] * CONFIG['gradient_accumulation_steps']}")
    print(f"  Learning rate: {CONFIG['learning_rate']}")
    print(f"  Max seq length: {CONFIG['max_seq_length']}")
    print()

    train_result = trainer.train()

    # Save final adapter weights
    adapter_dir = CHECKPOINT_DIR / "final-adapter"
    print(f"\nSaving adapter weights to {adapter_dir}")
    trainer.save_model(str(adapter_dir))
    tokenizer.save_pretrained(str(adapter_dir))

    # Save training metrics
    metrics = train_result.metrics
    metrics_path = TRAINING_DIR / "training_metrics.json"
    with open(metrics_path, "w") as f:
        json.dump(metrics, f, indent=2)
    print(f"Training metrics saved to {metrics_path}")

    return trainer


def evaluate(trainer):
    """Run evaluation after training."""
    print("\nRunning evaluation...")
    eval_results = trainer.evaluate()

    print("\nEvaluation Results:")
    for key, value in eval_results.items():
        print(f"  {key}: {value:.4f}" if isinstance(value, float) else f"  {key}: {value}")

    # Save eval results
    eval_path = TRAINING_DIR / "eval_results.json"
    with open(eval_path, "w") as f:
        json.dump(eval_results, f, indent=2)
    print(f"\nEval results saved to {eval_path}")

    return eval_results


def main():
    print("=" * 60)
    print("Zhen AI - RAFT QLoRA Fine-Tuning Pipeline")
    print("=" * 60)
    print()
    print(f"Base model:    {CONFIG['base_model']}")
    print(f"LoRA rank:     {CONFIG['lora_rank']}")
    print(f"LoRA alpha:    {CONFIG['lora_alpha']}")
    print(f"Target:        {', '.join(CONFIG['target_modules'])}")
    print(f"Batch size:    {CONFIG['per_device_train_batch_size']}")
    print(f"Grad accum:    {CONFIG['gradient_accumulation_steps']}")
    print(f"Epochs:        {CONFIG['num_train_epochs']}")
    print(f"Max seq len:   {CONFIG['max_seq_length']}")
    print(f"BF16:          {CONFIG['bf16']}")
    print()

    # Check dependencies
    has_bnb = check_dependencies()

    # Check GPU
    check_gpu()

    # Check training data exists
    if not TRAIN_PATH.exists() or not EVAL_PATH.exists():
        print("\nERROR: Training data not found.")
        print(f"  Expected: {TRAIN_PATH}")
        print(f"  Expected: {EVAL_PATH}")
        print("\nRun 07_prepare_training.py first.")
        sys.exit(1)

    # Load data
    train_dataset, eval_dataset = load_training_data()

    # Setup model
    model, tokenizer = setup_model_and_tokenizer(has_bnb)

    # Train
    trainer = train(model, tokenizer, train_dataset, eval_dataset)

    # Evaluate
    evaluate(trainer)

    print()
    print("=" * 60)
    print("Training complete!")
    print(f"Adapter weights: {CHECKPOINT_DIR / 'final-adapter'}")
    print("Next step: Run 09_merge_and_quantize.sh to create GGUF")
    print("=" * 60)


if __name__ == "__main__":
    main()
