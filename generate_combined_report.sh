#!/bin/bash

# Get output directory
output_dir="combined_report"
for arg in "$@"; do
  if [[ $arg == "--output" ]]; then
    output_next=true
  elif [[ $output_next == true ]]; then
    output_dir=$arg
    output_next=false
  fi
done

# Create output directory
mkdir -p "$output_dir"

# Create HTML header
cat > "$output_dir/combined_report.html" << EOF
<!DOCTYPE html>
<html>
<head>
  <title>Combined E-commerce Platform Benchmark Report</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 20px; }
    h1 { color: #333; }
    table { border-collapse: collapse; width: 100%; margin-top: 20px; }
    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
    th { background-color: #f2f2f2; }
    tr:nth-child(even) { background-color: #f9f9f9; }
  </style>
</head>
<body>
  <h1>Combined E-commerce Platform Benchmark Report</h1>
  <p>This report combines results from multiple benchmark durations.</p>
  
  <h2>Individual Reports</h2>
  <ul>
EOF

# Add links to individual reports
for dir in "$@"; do
  if [[ $dir != "--output" && $dir != $output_dir ]]; then
    echo "  <li><a href=\"../$dir/report.html\">$dir</a></li>" >> "$output_dir/combined_report.html"
  fi
done

# Close the HTML
cat >> "$output_dir/combined_report.html" << EOF
  </ul>
</body>
</html>
EOF

echo "Combined report generated at $output_dir/combined_report.html"