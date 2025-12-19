#!/bin/sh
# Profiling script for type_resolver optimizations
# Usage: ./profile.sh [baseline|optimized]

MODE=${1:-baseline}
OUTDIR="./profile_results"
mkdir -p "$OUTDIR"

echo "Running profiling in $MODE mode..."
echo "Output directory: $OUTDIR"
echo ""

cd /workspace/goscanner

# Run benchmarks with benchstat-friendly output
echo "1. Running benchmarks..."
go test -bench=. -run=^$ -benchmem -benchtime=3s -count=5 ./scanner/ > "$OUTDIR/${MODE}_bench.txt"

# CPU profile
echo "2. Generating CPU profile..."
go test -bench=BenchmarkTypeResolver_ResolveComplexPackage \
    -cpuprofile="$OUTDIR/${MODE}_cpu.prof" \
    -benchtime=3s \
    ./scanner/ > /dev/null 2>&1

# Memory profile
echo "3. Generating memory profile..."
go test -bench=BenchmarkTypeResolver_ResolveComplexPackage \
    -memprofile="$OUTDIR/${MODE}_mem.prof" \
    -benchtime=3s \
    ./scanner/ > /dev/null 2>&1

# Memory allocation profile
echo "4. Generating allocation profile..."
go test -bench=BenchmarkTypeResolver_ResolveComplexPackage \
    -memprofile="$OUTDIR/${MODE}_alloc.prof" \
    -benchtime=3s \
    ./scanner/ > /dev/null 2>&1

echo ""
echo "Profiling complete! Results in $OUTDIR/"
echo ""
echo "Quick stats from benchmarks:"
grep "^Benchmark" "$OUTDIR/${MODE}_bench.txt" | head -10
echo ""
echo "Next steps:"
echo "  - View CPU profile: go tool pprof $OUTDIR/${MODE}_cpu.prof"
echo "  - View memory profile: go tool pprof $OUTDIR/${MODE}_mem.prof"
if [ "$MODE" = "optimized" ] && [ -f "$OUTDIR/baseline_bench.txt" ]; then
    echo "  - Compare results: benchstat $OUTDIR/baseline_bench.txt $OUTDIR/optimized_bench.txt"
    echo "  - Compare CPU: go tool pprof -base=$OUTDIR/baseline_cpu.prof $OUTDIR/optimized_cpu.prof"
fi
