#!/usr/bin/env bash
# Measures xpm against the roadmap's performance budgets.
# Requires: hyperfine, jq. Network-dependent rows need internet access.
set -euo pipefail

command -v hyperfine >/dev/null || { echo "install hyperfine (brew install hyperfine)"; exit 2; }
command -v jq >/dev/null || { echo "install jq (brew install jq)"; exit 2; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$(mktemp -d)/xpm"
go build -trimpath -ldflags "-s -w" -o "$BIN" "$ROOT/cmd/xpm"
WORK="$(mktemp -d)"; cd "$WORK"   # empty dir: no project files, nothing to detect

# name | budget_ms | command
BUDGETS=(
  "help|10|$BIN help"
  "version|10|$BIN --version"
  "which-axios-cold|1500|env XPM_NO_CACHE=1 $BIN which axios"
  "which-typescript-cold|1500|env XPM_NO_CACHE=1 $BIN which typescript"
  "which-axios-warm|100|$BIN which axios"
)

fail=0
printf "%-20s %10s %10s  %s\n" "benchmark" "mean(ms)" "budget" "result"
for row in "${BUDGETS[@]}"; do
  IFS='|' read -r name budget cmd <<<"$row"
  hyperfine --warmup 2 --runs 10 --style none --export-json "$WORK/$name.json" "$cmd" >/dev/null 2>&1
  mean_ms=$(jq '.results[0].mean * 1000 | floor' "$WORK/$name.json")
  if [ "$mean_ms" -le "$budget" ]; then res="PASS"; else res="FAIL"; fail=1; fi
  printf "%-20s %10s %10s  %s\n" "$name" "$mean_ms" "$budget" "$res"
done

size_kb=$(( $(wc -c <"$BIN") / 1024 ))
# Stripped size: ~9.8 MB darwin/arm64, ~10.9 MB linux/amd64 (Go 1.27); was 14.6 MB unstripped.
if [ "$size_kb" -le 12288 ]; then res="PASS"; else res="FAIL"; fail=1; fi
printf "%-20s %10s %10s  %s\n" "binary-size(KB)" "$size_kb" "12288" "$res"

# Startup must not write files.
[ ! -e "$WORK/.xpm-env" ] || { echo "FAIL: startup wrote .xpm-env"; fail=1; }
exit $fail
