#!/bin/bash

# Define colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Define test durations in minutes
DURATIONS=(7 15 30)

# Create variable to store all result directories
ALL_RESULTS_DIRS=()

# Function to build the benchmark executables
build_benchmarks() {
  echo -e "${GREEN}Building benchmark executables...${NC}"
  
  # Build Medusa benchmark
  echo "Building Medusa benchmark..."
  (cd medusa && go build -o ../medusa_benchmark main.go)
  
  # Build Saleor benchmark
  echo "Building Saleor benchmark..."
  (cd saleor && go build -o ../saleor_benchmark main.go)
  
  # Build Spree benchmark
  echo "Building Spree benchmark..."
  (cd spree && go build -o ../spree_benchmark main.go)
  
  echo "All benchmark executables built successfully."
}

# Function to run a benchmark
run_benchmark() {
  local platform=$1
  local config=$2
  local results_dir=$3
  
  echo -e "${GREEN}Starting $platform benchmark...${NC}"
  
  # Run the benchmark and redirect output to a log file
  ./${platform}_benchmark -config $platform/$config > "$results_dir/${platform}_output.log" 2>&1
  
  # Check if the benchmark completed successfully
  if [ $? -eq 0 ]; then
    echo -e "${GREEN}$platform benchmark completed.${NC}"
    # Copy the results file to the results directory
    cp ${platform}_results.json "$results_dir/"
  else
    echo -e "${YELLOW}$platform benchmark failed. Check logs for details.${NC}"
  fi
}

# Function to run tests for a specific duration
run_tests_for_duration() {
  local duration=$1
  
  # Create results directory for this duration
  RESULTS_DIR="benchmark_results_${duration}min_$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$RESULTS_DIR"
  
  echo -e "${YELLOW}Running ${duration}-minute tests${NC}"
  echo -e "${YELLOW}=========================================================${NC}"
  echo "Results will be saved to: $RESULTS_DIR"
  echo
  
  # Create duration-specific config files
  for platform in medusa saleor spree; do
    # Calculate duration in nanoseconds
    DURATION_NS=$((duration * 60 * 1000000000))
    
    # Read the base config file
    CONFIG_FILE="${platform}/config.json"
    
    # Create a duration-specific config file
    DURATION_CONFIG="${platform}/config_${duration}min.json"
    
    # Update the Duration field in the config
    cat $CONFIG_FILE | sed "s/\"Duration\":[^,}]*/\"Duration\": $DURATION_NS/" > $DURATION_CONFIG
    
    echo "Created $DURATION_CONFIG with ${duration}-minute duration"
  done
  
  # Display benchmark configuration
  echo "Using configuration files for ${duration}-minute tests:"
  echo "- Medusa: medusa/config_${duration}min.json"
  echo "- Saleor: saleor/config_${duration}min.json"
  echo "- Spree: spree/config_${duration}min.json"
  echo
  
  # Run benchmarks in parallel
  echo -e "${GREEN}Running all benchmarks in parallel for ${duration} minutes...${NC}"
  run_benchmark medusa "config_${duration}min.json" "$RESULTS_DIR" &
  MEDUSA_PID=$!
  
  run_benchmark saleor "config_${duration}min.json" "$RESULTS_DIR" &
  SALEOR_PID=$!
  
  run_benchmark spree "config_${duration}min.json" "$RESULTS_DIR" &
  SPREE_PID=$!
  
  # Wait for all benchmarks to complete
  echo "Waiting for all ${duration}-minute benchmarks to complete..."
  wait $MEDUSA_PID
  wait $SALEOR_PID
  wait $SPREE_PID
  
  echo -e "${GREEN}All ${duration}-minute benchmarks completed.${NC}"
  
  # Compare the results for this duration
  echo -e "${GREEN}Comparing ${duration}-minute benchmark results...${NC}"
  go build -o compare_results compare_results.go
  ./compare_results \
    --medusa="$RESULTS_DIR/medusa_results.json" \
    --saleor="$RESULTS_DIR/saleor_results.json" \
    --spree="$RESULTS_DIR/spree_results.json" \
    --output="$RESULTS_DIR/comparison.json"
  
  # Generate HTML report for this duration
  echo -e "${GREEN}Generating HTML report for ${duration}-minute tests...${NC}"
  ./generate_report.sh "$RESULTS_DIR"
  
  echo -e "${GREEN}${duration}-minute benchmark suite completed.${NC}"
  echo "Results saved to: $RESULTS_DIR"
  echo "Report available at: $RESULTS_DIR/report.html"
  echo
  
  # Add this results directory to our list of all results
  ALL_RESULTS_DIRS+=("$RESULTS_DIR")
}

# Main execution starts here

# Build all benchmarks first
build_benchmarks

# Run tests for each duration
for duration in "${DURATIONS[@]}"; do
  run_tests_for_duration $duration
done

# Final combined report
echo -e "${GREEN}Generating combined report for all test durations...${NC}"
./generate_combined_report.sh "${ALL_RESULTS_DIRS[@]}" --output combined_report

echo -e "${GREEN}All benchmark testing completed.${NC}"
echo "Individual duration reports are available at:"
for dir in "${ALL_RESULTS_DIRS[@]}"; do
  echo "- $dir/report.html"
done
echo
echo "You can view the combined report at: combined_report/combined_report.html"