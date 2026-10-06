#!/usr/bin/env bash
# Measures xpm against the roadmap's performance budgets.
# Requires: hyperfine, jq. Network-dependent rows need internet access.
set -euo pipefail

command -v hyperfine >/dev/null || { echo "install hyperfine (brew install hyperfine)"; exit 2; }
command -v jq >/dev/null || { echo "install jq (brew install jq)"; exit 2; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$(mktemp -d)"
WORK="$(mktemp -d)"   # empty dir: no project files, nothing to detect
CACHE="$(mktemp -d)"  # private lookup cache: warm rows never touch the user's cache
trap 'rm -rf "$BIN_DIR" "$WORK" "$CACHE"' EXIT

BIN="$BIN_DIR/xpm"
go build -trimpath -ldflags "-s -w" -o "$BIN" "$ROOT/cmd/xpm"
cd "$WORK"

# name | budget_ms | command (run by hyperfine's shell, so paths are quoted)
BUDGETS=(
  "help|10|\"$BIN\" help"
  "version|10|\"$BIN\" --version"
  "which-axios-cold|1500|env XPM_NO_CACHE=1 \"$BIN\" which axios"
  "which-typescript-cold|1500|env XPM_NO_CACHE=1 \"$BIN\" which typescript"
  "which-axios-warm|100|env XPM_CACHE_DIR=\"$CACHE\" \"$BIN\" which axios"
  "search-axios-cold|1500|env XPM_NO_CACHE=1 \"$BIN\" search axios"
)

fail=0
printf "%-22s %10s %10s  %s\n" "benchmark" "mean(ms)" "budget" "result"
for row in "${BUDGETS[@]}"; do
  IFS='|' read -r name budget cmd <<<"$row"
  # A command that exits non-zero (offline, registry error) makes hyperfine
  # fail; report that as a FAIL row instead of letting set -e end the run.
  if ! out=$(hyperfine --warmup 2 --runs 10 --style none --export-json "$WORK/$name.json" "$cmd" 2>&1); then
    reason=$(printf '%s\n' "$out" | grep -v '^[[:space:]]*$' | tail -n 1) || true
    printf "%-22s %10s %10s  %s\n" "$name" "-" "$budget" "FAIL (${reason:-hyperfine failed})"
    fail=1
    continue
  fi
  mean_ms=$(jq '.results[0].mean * 1000 | floor' "$WORK/$name.json")
  if [ "$mean_ms" -le "$budget" ]; then res="PASS"; else res="FAIL"; fail=1; fi
  printf "%-22s %10s %10s  %s\n" "$name" "$mean_ms" "$budget" "$res"
done

size_kb=$(( $(wc -c <"$BIN") / 1024 ))
# Stripped size: ~9.8 MB darwin/arm64, ~10.9 MB linux/amd64 (Go 1.27); was 14.6 MB unstripped.
if [ "$size_kb" -le 12288 ]; then res="PASS"; else res="FAIL"; fail=1; fi
printf "%-22s %10s %10s  %s\n" "binary-size(KB)" "$size_kb" "12288" "$res"

# Startup must not write files.
[ ! -e "$WORK/.xpm-env" ] || { echo "FAIL: startup wrote .xpm-env"; fail=1; }
exit $fail
