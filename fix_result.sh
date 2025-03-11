#!/bin/bash

# Script to properly extract JSON results from benchmark log files

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
    
    # Check if log file exists
    if [ ! -f "$log_file" ]; then
      echo -e "${YELLOW}Warning: Log file not found: $log_file${NC}"
      continue
    fi
    
    # Extract JSON data from the log file - this looks for a JSON block at the end
    # Looking for the last occurrence of a line starting with {
    last_json_start=$(grep -n "^{" "$log_file" | tail -1 | cut -d: -f1)
    
    if [ -z "$last_json_start" ]; then
      echo -e "${YELLOW}Warning: No JSON data found in $log_file${NC}"
      continue
    fi
    
    # Extract from that line to the end or until a closing bracket
    json_data=$(tail -n +$last_json_start "$log_file" | sed -n '/^{/,/^}/p')
    
    if [ -z "$json_data" ]; then
      echo -e "${YELLOW}Warning: Could not extract valid JSON from $log_file${NC}"
      continue
    fi
    
    # Add platform name to the JSON data
    json_data=$(echo "$json_data" | sed '1s/{/{\"platform\":\"'$platform'\",/')
    
    # Write to result file
    echo "$json_data" > "$result_file"
    echo "Created result file: $result_file"
    
    # Validate JSON file
    if command -v jq >/dev/null 2>&1; then
      if jq . "$result_file" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ JSON is valid${NC}"
      else
        echo -e "${YELLOW}⚠ Warning: JSON in $result_file is not valid. Fixing...${NC}"
        # Get the raw content and try to fix it
        content=$(cat "$result_file")
        # Make sure it has proper closing bracket
        if [[ ! "$content" =~ \}$ ]]; then
          content="${content}}"
        fi
        echo "$content" > "$result_file"
        
        # Check again after fixing
        if jq . "$result_file" >/dev/null 2>&1; then
          echo -e "${GREEN}✓ JSON fixed successfully${NC}"
        else
          echo -e "${YELLOW}⚠ Could not fix JSON. Manual intervention required.${NC}"
        fi
      fi
    else
      echo "Note: jq not found, skipping JSON validation"
    fi
  done
done

echo -e "${GREEN}All results processed. You can now run generate_combined_report.sh${NC}"