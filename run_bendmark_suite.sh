#!/bin/bash

# Define colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Define test durations in minutes
DURATIONS=(7 15 30)

# Create variable to store all result directories
ALL_RESULTS_DIRS=()

# Function to run a benchmark with better error handling
run_benchmark() {
  local platform=$1
  local config=$2
  local results_dir=$3
  
  echo -e "${GREEN}Starting $platform benchmark...${NC}"
  
  # Run the benchmark and redirect output to a log file
  ./${platform}_benchmark -config $platform/$config > "$results_dir/${platform}_output.log" 2>&1 &
  local pid=$!
  echo "$platform PID: $pid"
  
  # Return the PID for later tracking
  echo $pid
}

# Function to wait for all benchmarks to complete with a timeout
wait_for_benchmarks() {
  local pids=("$@")
  local timeout=$((${DURATIONS[$duration_index]} * 60 + 300))  # Test duration + 5 minutes grace period
  local elapsed=0
  local interval=30  # Check every 30 seconds
  
  echo "Waiting for PIDs: ${pids[*]} with timeout of $timeout seconds"
  
  while [ $elapsed -lt $timeout ]; do
    # Check if any process is still running
    local all_done=true
    for pid in "${pids[@]}"; do
      if ps -p $pid > /dev/null 2>&1; then
        all_done=false
        break
      fi
    done
    
    if $all_done; then
      echo "All benchmark processes have completed"
      return 0
    fi
    
    echo "Waiting for benchmark processes to complete... ($elapsed / $timeout seconds)"
    sleep $interval
    elapsed=$((elapsed + interval))
  done
  
  # If we get here, we've timed out - kill any remaining processes
  echo "Timeout reached. Killing any remaining benchmark processes."
  for pid in "${pids[@]}"; do
    if ps -p $pid > /dev/null 2>&1; then
      echo "Killing process $pid"
      kill -9 $pid || true
    fi
  done
  
  return 1
}

# Function to run tests for a specific duration
run_tests_for_duration() {
  local duration=$1
  local duration_index=$2
  
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
  
  # Start all benchmarks and collect their PIDs
  MEDUSA_PID=$(run_benchmark medusa "config_${duration}min.json" "$RESULTS_DIR")
  SALEOR_PID=$(run_benchmark saleor "config_${duration}min.json" "$RESULTS_DIR")
  SPREE_PID=$(run_benchmark spree "config_${duration}min.json" "$RESULTS_DIR")
  
  # Wait for all benchmarks to complete
  wait_for_benchmarks $MEDUSA_PID $SALEOR_PID $SPREE_PID
  
  # Check for result files and copy them if they exist
  for platform in medusa saleor spree; do
    if [ -f "${platform}_results.json" ]; then
      echo "Copying ${platform}_results.json to $RESULTS_DIR/"
      cp ${platform}_results.json "$RESULTS_DIR/" || echo "Warning: Failed to copy ${platform}_results.json"
    else
      echo "${YELLOW}Warning: ${platform}_results.json not found${NC}"
    fi
  done
  
  echo -e "${GREEN}All ${duration}-minute benchmarks completed.${NC}"
  
  # Compare the results for this duration
  if [ -f "$RESULTS_DIR/medusa_results.json" ] && 
     [ -f "$RESULTS_DIR/saleor_results.json" ] && 
     [ -f "$RESULTS_DIR/spree_results.json" ]; then
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
  else
    echo "${YELLOW}Warning: Not all result files were found, skipping comparison generation${NC}"
  fi
  
  echo -e "${GREEN}${duration}-minute benchmark suite completed.${NC}"
  echo "Results saved to: $RESULTS_DIR"
  echo "Report available at: $RESULTS_DIR/report.html"
  echo
  
  # Add this results directory to our list of all results
  ALL_RESULTS_DIRS+=("$RESULTS_DIR")
}

# Main execution starts here

# Run tests for each duration sequentially
for i in "${!DURATIONS[@]}"; do
  run_tests_for_duration "${DURATIONS[$i]}" "$i"
done

# Final combined report
if [ ${#ALL_RESULTS_DIRS[@]} -gt 0 ]; then
  echo -e "${GREEN}Generating combined report for all test durations...${NC}"
  ./generate_combined_report.sh "${ALL_RESULTS_DIRS[@]}" --output combined_report

  echo -e "${GREEN}All benchmark testing completed.${NC}"
  echo "Individual duration reports are available at:"
  for dir in "${ALL_RESULTS_DIRS[@]}"; do
    echo "- $dir/report.html"
  done
  echo
  echo "You can view the combined report at: combined_report/combined_report.html"
else
  echo "${YELLOW}Warning: No test results were generated${NC}"
fi