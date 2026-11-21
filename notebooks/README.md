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

The repository carries no requirements file for the notebooks (FND-040, decided
2026-11-21): nothing in the stack or in CI runs them, and a lock would have put
114 packages into the dependency baseline. Run them from a throwaway environment
of your own, outside the repository and outside any venv a service uses, and
install there what the notebook you open imports (numpy, pandas, matplotlib,
seaborn, plotly, scipy, prometheus-api-client, psutil, nbformat, and Jupyter
itself). The versions last resolved for them are in git history
(`git log --diff-filter=D -- notebooks/requirements.in`).

```bash
python3 -m venv /var/tmp/unheaded-notebooks && . /var/tmp/unheaded-notebooks/bin/activate
pip install jupyterlab numpy pandas matplotlib seaborn plotly scipy prometheus-api-client psutil nbformat
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
