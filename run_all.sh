#!/bin/bash

# Define colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create results directory
RESULTS_DIR="benchmark_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo -e "${YELLOW}E-commerce Platform Performance Benchmark${NC}"
echo -e "${YELLOW}=======================================${NC}"
echo "Results will be saved to: $RESULTS_DIR"
echo

# Run Medusa benchmark
echo -e "${GREEN}Running Medusa benchmark...${NC}"
cd medusa
go build -o medusa_benchmark .
./medusa_benchmark
if [ $? -eq 0 ]; then
  cp medusa_results.json "../$RESULTS_DIR/"
  echo -e "${GREEN}Medusa benchmark completed successfully.${NC}"
else
  echo -e "${YELLOW}Medusa benchmark failed.${NC}"
fi
cd ..
echo

# Run Saleor benchmark
echo -e "${GREEN}Running Saleor benchmark...${NC}"
cd saleor
go build -o saleor_benchmark .
./saleor_benchmark
if [ $? -eq 0 ]; then
  cp saleor_results.json "../$RESULTS_DIR/"
  echo -e "${GREEN}Saleor benchmark completed successfully.${NC}"
else
  echo -e "${YELLOW}Saleor benchmark failed.${NC}"
fi
cd ..
echo

# Run Spree benchmark
echo -e "${GREEN}Running Spree benchmark...${NC}"
cd spree
go build -o spree_benchmark .
./spree_benchmark
if [ $? -eq 0 ]; then
  cp spree_results.json "../$RESULTS_DIR/"
  echo -e "${GREEN}Spree benchmark completed successfully.${NC}"
else
  echo -e "${YELLOW}Spree benchmark failed.${NC}"
fi
cd ..
echo

# Compare the results
echo -e "${GREEN}Comparing benchmark results...${NC}"
go run compare_results.go \
  --medusa="$RESULTS_DIR/medusa_results.json" \
  --saleor="$RESULTS_DIR/saleor_results.json" \
  --spree="$RESULTS_DIR/spree_results.json" \
  --output="$RESULTS_DIR/comparison.json"

# Generate HTML report
echo -e "${GREEN}Generating HTML report...${NC}"
go run generate_report.go \
  --input="$RESULTS_DIR/comparison.json" \
  --output="$RESULTS_DIR/report.html"

echo -e "${GREEN}Benchmark suite completed.${NC}"
echo "Results saved to: $RESULTS_DIR"
echo "Full comparison report available at: $RESULTS_DIR/report.html"