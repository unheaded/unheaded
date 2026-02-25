# Unheaded — Scientific Lab Notebooks

PhD-level experimental design. Synthetic fallback when bare metal is offline.
Re-run with live data when hosts are online.

## Notebooks

| # | File | Purpose |
|---|------|---------|
| 01 | `01_system_baseline.ipynb` | System characterization — CPU, RAM, disk, net, process inventory |
| 02 | `02_hypothesis_matrix.ipynb` | H1–H8 falsifiable hypothesis tests with bootstrap CI |
| 03 | `03_ebpf_latency_analysis.ipynb` | XDP/TC latency deep dive, per-hop analysis, BPF map pressure |
| 04 | `04_wireguard_analysis.ipynb` | WireGuard east-west RTT, throughput, crypto overhead |
| 05 | `05_llm_inference_analysis.ipynb` | vLLM + ROCm tok/s, VRAM, resource contention under load |

## Usage

```bash
pip install -r requirements.txt
jupyter lab
```

## Live vs Synthetic Mode

Each notebook auto-detects bare metal vs sandbox:
- **Synthetic**: Runs everywhere, generates statistically realistic simulated data
- **Live**: Reads from `/proc`, `Prometheus :9090`, `rocm-smi`, `wg show`

To force live mode: set `UNHEADED_LIVE=1` in environment.

## Re-run Protocol (bare metal)

1. Boot hosts, start all services + node_exporter + Prometheus
2. `export UNHEADED_LIVE=1 PROMETHEUS_URL=http://localhost:9090`
3. `jupyter nbconvert --to notebook --execute *.ipynb`
4. Commit results: `git add notebooks/executed/ && git commit -m "data: bare metal baseline YYYY-MM-DD"`

## Dark Theme

All plots use the Kingdom palette: `#0a0a0a` background, `#00ff88` accent.
