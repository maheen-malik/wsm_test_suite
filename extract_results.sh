#!/bin/bash

# Script to extract JSON results from benchmark log files

# Set colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Process each results directory
for dir in benchmark_results_*min_*; do
  echo -e "${GREEN}Processing directory: $dir${NC}"
  
  # Process each platform's log file
  for platform in medusa saleor spree; do
    log_file="$dir/${platform}_output.log"
    result_file="$dir/${platform}_results.json"
    
    # Skip if result file already exists
    if [ -f "$result_file" ]; then
      echo "Result file already exists: $result_file"
      continue
    fi
    
    # Check if log file exists
    if [ ! -f "$log_file" ]; then
      echo -e "${YELLOW}Warning: Log file not found: $log_file${NC}"
      continue
    fi
    
    # Extract JSON data from the log file
    # This assumes JSON data is at the end of the file and starts with a {
    json_data=$(grep -A 50 -m 1 "^{" "$log_file" | sed -n '/^{/,/^}/p')
    
    if [ -z "$json_data" ]; then
      echo -e "${YELLOW}Warning: No JSON data found in $log_file${NC}"
      continue
    fi
    
    # Add platform name to the JSON data
    json_data=$(echo "$json_data" | sed '1s/{/{\"platform\":\"'$platform'\",/')
    
    # Write to result file
    echo "$json_data" > "$result_file"
    echo "Created result file: $result_file"
  done
done

echo -e "${GREEN}All results extracted. You can now run generate_combined_report.sh${NC}"