#!/usr/bin/env bash
# Run the problem-quality benchmark from a preprod/CI runner.
#
# Secrets are expected from the environment (normally injected by Doppler):
#   DAYTONA_API_KEY, DAYTONA_API_URL, CODEGYM_BASE_SNAPSHOT,
#   CODEGYM_GENAI_PROVIDER_ORDER, provider API keys.
#
# Optional inputs:
#   SMOKE=true                 run only the Python regression case
#   BASELINE_KIND=...          passed to -baseline-kind
#   MODEL_REVISION=...         passed to -model-revision
#   PROMPT_REVISION=...        passed to -prompt-revision
#   PQ_BUDGET_STATE=...        budget ledger path (blank disables accounting)
#   PQ_PRIOR_SPEND_MICROS=...  prior spend when the ledger is first created
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/backend"

RUN_ID="pqb-$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -tx1 -N4 /dev/urandom 2>/dev/null | tr -d ' \n' | cut -c1-8)"
if [[ -z "${RUN_ID##*-}" ]]; then
  RUN_ID="${RUN_ID}$(date +%s | tail -c 5)"
fi
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "run_id=$RUN_ID" >> "$GITHUB_OUTPUT"
fi

SMOKE="${SMOKE:-false}"
BASELINE_KIND="${BASELINE_KIND:-current}"
MODEL_REVISION="${MODEL_REVISION:-}"
PROMPT_REVISION="${PROMPT_REVISION:-}"
BUDGET_STATE="${PQ_BUDGET_STATE:-$ROOT/budget/problem-quality-budget.json}"
PRIOR_SPEND_MICROS="${PQ_PRIOR_SPEND_MICROS:-0}"
GENERATION_PROVIDER="${GENERATION_PROVIDER:-meta}"
SKIP_GENERATION="${SKIP_GENERATION:-false}"

CASES=(
  "-case" "../benchmarks/problem-quality/cases/python-sum-even-numbers"
)
if [[ "$SMOKE" != "true" ]]; then
  CASES+=(
    "-case" "../benchmarks/problem-quality/cases/go-count-even-numbers"
    "-case" "../benchmarks/problem-quality/cases/express-greeting-endpoint"
    "-case" "../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint"
  )
fi

OUT_DIR="$ROOT/benchmark-results/preprod/$RUN_ID"
mkdir -p "$OUT_DIR"

BUDGET_ARGS=()
if [[ -n "$BUDGET_STATE" ]]; then
  mkdir -p "$(dirname "$BUDGET_STATE")"
  BUDGET_ARGS=(-budget-state "$BUDGET_STATE" -prior-spend-micros "$PRIOR_SPEND_MICROS")
else
  BUDGET_ARGS=(-budget-state '')
fi

GENERATION_ARGS=()
if [[ "$SKIP_GENERATION" != "true" ]]; then
  GENERATION_ARGS=(-generate-reference -generation-provider "$GENERATION_PROVIDER")
fi

go run ./cmd/problem-quality-benchmark \
  -execution sandbox \
  "${GENERATION_ARGS[@]}" \
  "${CASES[@]}" \
  -baseline-kind "$BASELINE_KIND" \
  -model-revision "$MODEL_REVISION" \
  -prompt-revision "$PROMPT_REVISION" \
  -review-authority hunter \
  -review-authority cristina \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -output "$OUT_DIR/report.json" \
  -timeout 300s \
  "${BUDGET_ARGS[@]}"

cat > "$OUT_DIR/README.md" <<EOF
# Problem-quality preprod run: $RUN_ID

Preliminary preprod evidence. Assessment ran in Daytona sandboxes.
Generation baseline: $([[ "$SKIP_GENERATION" == "true" ]] && echo "skipped (debug run)" || echo "$GENERATION_PROVIDER")

- Smoke run: $SMOKE
- Baseline kind: $BASELINE_KIND
- Model revision: ${MODEL_REVISION:-not supplied}
- Prompt revision: ${PROMPT_REVISION:-not supplied}
- Review authorities: hunter, cristina
- Budget state: ${BUDGET_STATE:-disabled}
- Report: \`report.json\`

This run is preliminary. It does not prove broad trustworthiness and does not
measure blind agreement/reconciliation until independent generation is merged.
EOF

echo "run_id=$RUN_ID"
