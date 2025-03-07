#!/bin/bash

# Define colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create results directory
RESULTS_DIR="benchmark_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo -e "${YELLOW}E-commerce Platform Performance Benchmark (Parallel Execution)${NC}"
echo -e "${YELLOW}=========================================================${NC}"
echo "Results will be saved to: $RESULTS_DIR"
echo

# Function to build the benchmark executables
build_benchmarks() {
  echo -e "${GREEN}Building benchmark executables...${NC}"
  
  # Build Medusa benchmark
  (cd medusa && go build -o medusa_benchmark)
  
  # Build Saleor benchmark
  (cd saleor && go build -o saleor_benchmark)
  
  # Build Spree benchmark
  (cd spree && go build -o spree_benchmark)
  
  echo "All benchmark executables built successfully."
}

# Function to run a benchmark in the background
run_benchmark() {
  local platform=$1
  local config=$2
  
  echo -e "${GREEN}Starting $platform benchmark...${NC}"
  
  # Run the benchmark and redirect output to a log file
  (cd $platform && ./${platform}_benchmark -config $config > "../$RESULTS_DIR/${platform}_output.log" 2>&1)
  
  # Check if the benchmark completed successfully
  if [ $? -eq 0 ]; then
    echo -e "${GREEN}$platform benchmark completed.${NC}"
    # Copy the results file to the results directory
    cp $platform/${platform}_results.json "$RESULTS_DIR/"
  else
    echo -e "${YELLOW}$platform benchmark failed. Check logs for details.${NC}"
  fi
}

# Build all benchmarks first
build_benchmarks

# Display benchmark configuration
echo "Using configuration files:"
echo "- Medusa: medusa/config.json"
echo "- Saleor: saleor/config.json"
echo "- Spree: spree/config.json"
echo

# Run benchmarks in parallel
echo -e "${GREEN}Running all benchmarks in parallel...${NC}"
run_benchmark medusa config.json &
MEDUSA_PID=$!

run_benchmark saleor config.json &
SALEOR_PID=$!

run_benchmark spree config.json &
SPREE_PID=$!

# Wait for all benchmarks to complete
echo "Waiting for all benchmarks to complete..."
wait $MEDUSA_PID
wait $SALEOR_PID
wait $SPREE_PID

echo -e "${GREEN}All benchmarks completed.${NC}"

# Compare the results
echo -e "${GREEN}Comparing benchmark results...${NC}"
go build -o compare_results compare_results.go
./compare_results \
  --medusa="$RESULTS_DIR/medusa_results.json" \
  --saleor="$RESULTS_DIR/saleor_results.json" \
  --spree="$RESULTS_DIR/spree_results.json" \
  --output="$RESULTS_DIR/comparison.json"

echo -e "${GREEN}Benchmark suite completed.${NC}"
echo "Results saved to: $RESULTS_DIR"
echo "Comparison report available at: $RESULTS_DIR/comparison.json"
echo
echo "You can view the individual benchmark logs at:"
echo "- $RESULTS_DIR/medusa_output.log"
echo "- $RESULTS_DIR/saleor_output.log"
echo "- $RESULTS_DIR/spree_output.log"