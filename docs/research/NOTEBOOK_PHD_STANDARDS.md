# Unheaded Scientific Notebooks — PhD Thesis Standards

## Mandate
These notebooks are NOT demos. They are pre-registered scientific experiments.
Strong Inference methodology (Platt 1964). Every hypothesis falsifiable before data collection.
Bootstrap CI (N=10,000) on all comparisons. α=0.05 throughout.

## Structure Required in Each Notebook

### 1. Abstract (≤ 200 words)
- What was measured
- Method summary
- Key quantitative finding
- Significance

### 2. Hypothesis (pre-registered)
- Null hypothesis (H₀)
- Alternative hypothesis (H₁)
- Rejection criterion (α, CI bounds)
- Expected effect size (Cohen's d or relative %)

### 3. Experimental Design
- Independent variables
- Dependent variables
- Controlled variables
- Sample size justification (power analysis)
- Confounds acknowledged

### 4. Data Collection
- `UNHEADED_LIVE=1`: real hardware data
- `UNHEADED_LIVE=0`: statistically-realistic synthetic (document distribution parameters)
- Provenance: timestamp, host, kernel version, git commit

### 5. Statistical Analysis
- Bootstrap CI (N=10,000) for all key metrics
- P-value where applicable (Welch's t-test, Mann-Whitney U for non-normal)
- Effect size (Cohen's d for continuous, Cliff's delta for ordinal)
- Multiple comparisons: Bonferroni correction where testing H1-H8 simultaneously

### 6. Results
- Tables: mean ± std, [CI_low, CI_high], p-value, effect size
- Figures: must include error bars/CI bands, not just point estimates
- No cherry-picking: report all outcomes including failures

### 7. Discussion
- Does result confirm or falsify the hypothesis?
- Alternative explanations considered
- Limitations (synthetic vs live, sample size, hardware variance)
- Implications for architecture decisions

### 8. Verdict
```
H1: [CONFIRMED|FALSIFIED|INCONCLUSIVE]
Measured: [value] [CI low, CI high]
Threshold: [criterion]
```

## Plot Standards
- Background: #0a0a0a (Kingdom black)
- Primary data: #00ff88 (Kingdom green)
- Secondary data: #3d9be9 (info blue)
- Warning bands: #ff6b35 (warning orange)
- Critical thresholds: #ff2442 (critical red)
- Font: monospace preferred (Courier New, DejaVu Mono)
- All axes: labeled with units
- All figures: title, subtitle with N and CI level
- DPI: 150 for inline, 300 for publication

## Anti-patterns Forbidden
- Histograms without CI bands
- P-values without effect sizes
- "Looks good" or "seems fast" as conclusions
- Graphs without axis labels or units
- Claiming significance without correcting for multiple comparisons
- Synthetic data presented as if real (must be labeled)

## Notebook Index

| # | Notebook | Primary Hypothesis | Key Metric | Status |
|---|----------|--------------------|------------|--------|
| 01 | system_baseline | N/A (descriptive) | CPU/RAM/IO inventory | Synthetic |
| 02 | hypothesis_matrix | H1-H8 omnibus | Bootstrap CI on all 8 | Synthetic |
| 03 | ebpf_latency | H1: XDP P99 < 1µs | flow_latency_ns P99 | Synthetic |
| 04 | wireguard | H4: RTT P99 < 5ms | RTT histogram | Synthetic |
| 05 | llm_inference | H5: tok/s > 30 | tokens_per_second | Synthetic |

## Re-run Protocol
1. Set UNHEADED_LIVE=1 PROMETHEUS_URL=http://localhost:9090
2. jupyter nbconvert --to notebook --execute *.ipynb --output-dir executed/
3. Commit executed/ with: git add notebooks/executed/ && git commit -m "data: live YYYY-MM-DD"
4. All executed notebooks must be in a separate executed/ subdirectory (never overwrite source)
