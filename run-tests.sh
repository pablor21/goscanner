#!/usr/bin/env sh
set -eu

# run-tests.sh — convenience runner for Go tests in this repo
# Usage:
#   ./run-tests.sh [tests|profiling|all] [extra go test flags...]
# Defaults to 'tests'.

MODE="${1:-tests}"
if [ $# -gt 0 ]; then
  shift
fi

case "$MODE" in
  tests)
    echo "[run-tests] Mode=tests (quick unit tests only)."
    # Run only fast, deterministic unit tests; exclude long real-world/profile tests
    go test "$@" ./scanner -run '^(TestDeterministicSerialization|TestTypesFullyLoadedBeforeSerialize)$'
    ;;
  profiling)
    echo "[run-tests] Mode=profiling (only TestProfile*, enabling PROFILE=1)."
    PROFILE=1 go test "$@" ./... -run '^TestProfile.*$'
    ;;
  all)
    echo "[run-tests] Mode=all (running everything in scanner, enabling PROFILE=1)."
    PROFILE=1 go test "$@" ./scanner -v
    ;;
  *)
    echo "Usage: $0 [tests|profiling|all] [extra go test flags...]" >&2
    exit 2
    ;;
esac
