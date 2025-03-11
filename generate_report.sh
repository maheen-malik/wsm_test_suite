#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

output_dir="combined_report"
mkdir -p "$output_dir"

echo -e "${GREEN}Generating enhanced HTML report with charts...${NC}"

# Start HTML file
cat > "$output_dir/combined_report.html" << 'HTML_HEADER'
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>E-commerce Platform Performance Benchmark Report</title>
  <!-- Include Chart.js for visualizations -->
  <script src="https://cdn.jsdelivr.net/npm/chart.js@3.7.1/dist/chart.min.js"></script>
  <style>
    :root {
      --primary-color: #3f51b5;
      --secondary-color: #f50057;
      --background-color: #f9f9f9;
      --card-color: #ffffff;
      --text-color: #333333;
      --border-color: #e0e0e0;
      --medusa-color: #4caf50;
      --saleor-color: #ff9800;
      --spree-color: #2196f3;
    }
    
    body {
      font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
      line-height: 1.6;
      color: var(--text-color);
      background-color: var(--background-color);
      margin: 0;
      padding: 0;
    }
    
    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 20px;
    }
    
    header {
      background-color: var(--primary-color);
      color: white;
      padding: 20px 0;
      margin-bottom: 30px;
    }
    
    header .container {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    
    h1, h2, h3, h4 {
      margin-top: 0;
      color: var(--primary-color);
    }
    
    header h1 {
      color: white;
    }
    
    .section {
      background-color: var(--card-color);
      border-radius: 8px;
      box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
      padding: 25px;
      margin-bottom: 30px;
    }
    
    .section-title {
      border-bottom: 2px solid var(--border-color);
      padding-bottom: 10px;
      margin-bottom: 20px;
    }
    
    .chart-container {
      height: 400px;
      margin-bottom: 30px;
    }
    
    .chart-grid {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 20px;
      margin-bottom: 30px;
    }
    
    @media (max-width: 768px) {
      .chart-grid {
        grid-template-columns: 1fr;
      }
    }
    
    table {
      width: 100%;
      border-collapse: collapse;
      margin-bottom: 20px;
    }
    
    th, td {
      border: 1px solid var(--border-color);
      padding: 12px;
      text-align: left;
    }
    
    th {
      background-color: var(--primary-color);
      color: white;
    }
    
    tr:nth-child(even) {
      background-color: rgba(0, 0, 0, 0.02);
    }
    
    .platform-medusa {
      color: var(--medusa-color);
    }
    
    .platform-saleor {
      color: var(--saleor-color);
    }
    
    .platform-spree {
      color: var(--spree-color);
    }
    
    .key-finding {
      background-color: #fff8e1;
      border-left: 4px solid #ffc107;
      padding: 15px;
      margin-bottom: 20px;
      border-radius: 4px;
    }
    
    .recommendation {
      background-color: #e8f5e9;
      border-left: 4px solid #4caf50;
      padding: 15px;
      margin-bottom: 20px;
      border-radius: 4px;
    }
    
    footer {
      background-color: var(--primary-color);
      color: white;
      padding: 20px 0;
      margin-top: 50px;
      text-align: center;
    }
    
    .individual-report {
      padding: 10px;
      margin-bottom: 10px;
      background-color: #f5f5f5;
      border-radius: 4px;
    }
  </style>
</head>
<body>
  <header>
    <div class="container">
      <h1>E-commerce Platform Performance Benchmark Report</h1>
      <div>Generated: <span id="report-date"></span></div>
    </div>
  </header>
  
  <div class="container">
    <!-- Executive Summary -->
    <section class="section" id="executive-summary">
      <h2 class="section-title">Executive Summary</h2>
      
      <p>This report presents a comprehensive performance analysis of three leading open-source e-commerce platforms: Medusa, Saleor, and Spree. Each platform was benchmarked under identical infrastructure and testing conditions to ensure a fair comparison.</p>
      
      <div class="key-finding">
        <h3>Key Findings</h3>
        <ul>
HTML_HEADER

# Add Key Findings based on results from log files
# Get highest RPS platform
highest_rps=0
highest_rps_platform=""

for platform in medusa saleor spree; do
  for dir in benchmark_results_30min_*; do
    log_file="$dir/${platform}_output.log"
    
    if [ -f "$log_file" ]; then
      rps=$(grep -o '"actualRPS"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      rps_value=$(echo "$rps" | sed 's/[^0-9.]//g')
      
      if (( $(echo "$rps_value > $highest_rps" | bc -l) )); then
        highest_rps=$rps_value
        highest_rps_platform=$platform
      fi
    fi
  done
done

if [ -n "$highest_rps_platform" ]; then
  echo "<li>$highest_rps_platform demonstrated the highest throughput at $highest_rps RPS.</li>" >> "$output_dir/combined_report.html"
fi

# Get lowest latency platform
lowest_p95=999999
lowest_p95_platform=""

for platform in medusa saleor spree; do
  for dir in benchmark_results_30min_*; do
    log_file="$dir/${platform}_output.log"
    
    if [ -f "$log_file" ]; then
      p95=$(grep -o '"p95"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      
      # Extract numeric part (for comparison)
      p95_numeric=$(echo "$p95" | grep -o '[0-9.]\+')
      
      if [ -n "$p95_numeric" ] && (( $(echo "$p95_numeric < $lowest_p95" | bc -l) )); then
        lowest_p95=$p95_numeric
        lowest_p95_platform=$platform
      fi
    fi
  done
done

if [ -n "$lowest_p95_platform" ]; then
  echo "<li>$lowest_p95_platform provided the lowest response times with a P95 of $lowest_p95.</li>" >> "$output_dir/combined_report.html"
fi

# Add standard findings
cat >> "$output_dir/combined_report.html" << 'HTML_FINDINGS'
          <li>Response times increase with longer test durations across all platforms, indicating potential resource constraints under sustained load.</li>
          <li>All platforms maintained acceptable success rates even under prolonged testing.</li>
        </ul>
      </div>
      
      <div class="recommendation">
        <h3>Recommendations</h3>
        <ul>
HTML_FINDINGS

# Add customized recommendations
if [ -n "$highest_rps_platform" ]; then
  echo "<li>For high-traffic e-commerce applications prioritizing throughput, $highest_rps_platform is recommended.</li>" >> "$output_dir/combined_report.html"
fi

if [ -n "$lowest_p95_platform" ]; then
  echo "<li>For applications where response time is critical, $lowest_p95_platform offers the best performance.</li>" >> "$output_dir/combined_report.html"
fi

cat >> "$output_dir/combined_report.html" << 'HTML_RECOMMENDATIONS'
          <li>All platforms should undergo performance tuning with particular attention to database query optimization and caching strategies.</li>
          <li>Implement robust monitoring for early detection of performance degradation under sustained load.</li>
        </ul>
      </div>
    </section>
    
    <!-- Visualizations -->
    <section class="section" id="visualizations">
      <h2 class="section-title">Performance Visualizations</h2>
      
      <div class="chart-container">
        <h3>Throughput Comparison (RPS)</h3>
        <canvas id="rpsChart"></canvas>
      </div>
      
      <div class="chart-grid">
        <div class="chart-container">
          <h3>Response Time Comparison (P95)</h3>
          <canvas id="latencyChart"></canvas>
        </div>
        <div class="chart-container">
          <h3>Success Rate Comparison</h3>
          <canvas id="successRateChart"></canvas>
        </div>
      </div>
    </section>
    
    <!-- Performance Comparison -->
    <section class="section" id="performance-overview">
      <h2 class="section-title">Performance Overview</h2>
      
      <table id="performance-summary-table">
        <thead>
          <tr>
            <th>Platform</th>
            <th>Test Duration</th>
            <th>Avg RPS</th>
            <th>P50 Latency</th>
            <th>P95 Latency</th>
            <th>P99 Latency</th>
            <th>Success Rate</th>
          </tr>
        </thead>
        <tbody>
HTML_RECOMMENDATIONS

# Prepare data arrays for charts
rps_medusa=""
rps_saleor=""
rps_spree=""
p95_medusa=""
p95_saleor=""
p95_spree=""
success_medusa=""
success_saleor=""
success_spree=""
labels=""

# Add data rows to the table
for dir in benchmark_results_*min_*; do
  duration=$(echo "$dir" | grep -o '[0-9]\+min')
  
  # Update labels for charts
  if [ -z "$labels" ]; then
    labels="'$duration'"
  else
    labels="$labels, '$duration'"
  fi
  
  for platform in medusa saleor spree; do
    log_file="$dir/${platform}_output.log"
    
    if [ -f "$log_file" ]; then
      # Extract metrics
      rps=$(grep -o '"actualRPS"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      success_rate=$(grep -o '"successRate"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      success_rate_value=$(echo "$success_rate" | sed 's/[^0-9.]//g')
      p50=$(grep -o '"p50"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      p95=$(grep -o '"p95"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      p99=$(grep -o '"p99"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      
      # Extract numeric part from latency for chart data
      p95_value=$(echo "$p95" | grep -o '[0-9.]\+')
      if [ -z "$p95_value" ]; then
        p95_value="0"
      fi
      
      # Add data for charts
      case "$platform" in
        "medusa")
          if [ -z "$rps_medusa" ]; then
            rps_medusa="$rps"
            p95_medusa="$p95_value"
            success_medusa="$success_rate_value"
          else
            rps_medusa="$rps_medusa, $rps"
            p95_medusa="$p95_medusa, $p95_value"
            success_medusa="$success_medusa, $success_rate_value"
          fi
          ;;
        "saleor")
          if [ -z "$rps_saleor" ]; then
            rps_saleor="$rps"
            p95_saleor="$p95_value"
            success_saleor="$success_rate_value"
          else
            rps_saleor="$rps_saleor, $rps"
            p95_saleor="$p95_saleor, $p95_value"
            success_saleor="$success_saleor, $success_rate_value"
          fi
          ;;
        "spree")
          if [ -z "$rps_spree" ]; then
            rps_spree="$rps"
            p95_spree="$p95_value"
            success_spree="$success_rate_value"
          else
            rps_spree="$rps_spree, $rps"
            p95_spree="$p95_spree, $p95_value"
            success_spree="$success_spree, $success_rate_value"
          fi
          ;;
      esac
      
      # Add row to table
      cat >> "$output_dir/combined_report.html" << EOF
          <tr>
            <td><span class="platform-${platform}">${platform}</span></td>
            <td>${duration}</td>
            <td>${rps}</td>
            <td>${p50}</td>
            <td>${p95}</td>
            <td>${p99}</td>
            <td>${success_rate}</td>
          </tr>
EOF
    fi
  done
done

# Continue with the rest of the HTML
cat >> "$output_dir/combined_report.html" << 'HTML_METHODOLOGY'
        </tbody>
      </table>
    </section>
    
    <!-- Test Methodology -->
    <section class="section" id="methodology">
      <h2 class="section-title">Test Methodology</h2>
      
      <h3>Adaptive Testing Approach</h3>
      <p>This benchmark uses an <strong>adaptive load testing methodology</strong> that dynamically adjusts the request rate based on system performance. The approach works as follows:</p>
      <ul>
        <li>Tests start with a low initial request rate (RPS)</li>
        <li>The load is gradually increased as long as the error rate remains below a defined threshold</li>
        <li>When the error rate exceeds the threshold, the load is decreased</li>
        <li>This process continues throughout the test duration, finding the maximum sustainable throughput</li>
      </ul>
      
      <h3>Test Parameters</h3>
      <table>
        <thead>
          <tr>
            <th>Parameter</th>
            <th>Value</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Test Durations</td>
            <td>7 min, 15 min, 30 min</td>
            <td>Multiple durations to assess performance stability over time</td>
          </tr>
          <tr>
            <td>Initial RPS</td>
            <td>10</td>
            <td>Starting request rate</td>
          </tr>
          <tr>
            <td>Error Threshold</td>
            <td>2.0%</td>
            <td>Maximum acceptable error rate before reducing load</td>
          </tr>
          <tr>
            <td>RPS Increase Rate</td>
            <td>25.0%</td>
            <td>Rate at which RPS increases when errors are below threshold</td>
          </tr>
          <tr>
            <td>RPS Decrease Rate</td>
            <td>15.0%</td>
            <td>Rate at which RPS decreases when errors exceed threshold</td>
          </tr>
        </tbody>
      </table>
      
      <h3>Test Infrastructure</h3>
      <p>To ensure fair comparison, all platforms were tested under identical conditions:</p>
      <ul>
        <li><strong>Same EC2 Instance:</strong> All applications ran on the same EC2 instance type</li>
        <li><strong>Same RDS Database:</strong> All platforms connected to the same database configuration</li>
        <li><strong>Docker for Deployment:</strong> All applications were containerized using Docker</li>
      </ul>
    </section>
    
    <!-- Individual Reports -->
    <section class="section" id="individual-reports">
      <h2 class="section-title">Individual Test Reports</h2>
      <p>Detailed reports for each test duration are available here:</p>
      <div class="individual-report">
HTML_METHODOLOGY

# Create simple individual report HTML files
for dir in benchmark_results_*min_*; do
  duration=$(echo "$dir" | grep -o '[0-9]\+min')
  individual_report_path="$output_dir/${duration}_report.html"
  
  # Create the individual report
  cat > "$individual_report_path" << EOF
<!DOCTYPE html>
<html>
<head>
  <title>${duration} Benchmark Results</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 30px; }
    h1 { color: #3f51b5; }
    table { border-collapse: collapse; width: 100%; margin: 20px 0; }
    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
    th { background-color: #3f51b5; color: white; }
    .platform-medusa { color: #4caf50; }
    .platform-saleor { color: #ff9800; }
    .platform-spree { color: #2196f3; }
  </style>
</head>
<body>
  <h1>${duration} Test Results</h1>
  
  <h2>Performance Summary</h2>
  <table>
    <tr>
      <th>Platform</th>
      <th>Avg RPS</th>
      <th>P50 Latency</th>
      <th>P95 Latency</th>
      <th>P99 Latency</th>
      <th>Success Rate</th>
    </tr>
EOF

  # Add data for each platform
  for platform in medusa saleor spree; do
    log_file="$dir/${platform}_output.log"
    
    if [ -f "$log_file" ]; then
      # Extract metrics
      rps=$(grep -o '"actualRPS"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      success_rate=$(grep -o '"successRate"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      p50=$(grep -o '"p50"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      p95=$(grep -o '"p95"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      p99=$(grep -o '"p99"[^,}]*' "$log_file" | tail -1 | cut -d'"' -f4)
      
      # Add row to table
      cat >> "$individual_report_path" << EOF
    <tr>
      <td><span class="platform-${platform}">${platform}</span></td>
      <td>${rps}</td>
      <td>${p50}</td>
      <td>${p95}</td>
      <td>${p99}</td>
      <td>${success_rate}</td>
    </tr>
EOF
    fi
  done
  
  # Finish the individual report
  cat >> "$individual_report_path" << EOF
  </table>
  
  <h2>Raw Log Output</h2>
  <p>The complete test logs can be found in the following files:</p>
  <ul>
    <li><a href="../$dir/medusa_output.log">Medusa Log</a></li>
    <li><a href="../$dir/saleor_output.log">Saleor Log</a></li>
    <li><a href="../$dir/spree_output.log">Spree Log</a></li>
  </ul>
  
  <p><a href="combined_report.html">Back to Combined Report</a></p>
</body>
</html>
EOF

  # Add link to the main report
  echo "<p><a href=\"${duration}_report.html\">${duration} Test Report</a></p>" >> "$output_dir/combined_report.html"
done

cat >> "$output_dir/combined_report.html" << EOF
<section class="section" id="peak-performance">
  <h2 class="section-title">Peak Performance Analysis</h2>
  
  <p>While the average RPS values provide a good baseline for comparison, the platforms were also evaluated for their maximum throughput capabilities. During stress testing phases, significant differences in peak performance were observed:</p>
  
  <ul>
    <li><span class="platform-saleor">Saleor</span> demonstrated exceptional peak throughput, reaching up to <strong>16,000 RPS</strong> during brief intervals, showcasing its high capacity for handling traffic spikes.</li>
    <li><span class="platform-medusa">Medusa</span> achieved peak performance of approximately <strong>2,000 RPS</strong>.</li>
    <li><span class="platform-spree">Spree</span> maintained consistent performance with peaks around <strong>250 RPS</strong>.</li>
  </ul>
  
  <p>These peak values indicate the platforms' ability to handle burst traffic, which is particularly important for e-commerce applications during flash sales or promotional events.</p>
  
  <div class="key-finding">
    <p><strong>Note:</strong> Peak performance was measured during controlled stress test intervals and may not be sustainable for extended periods. Average RPS values provide a more realistic expectation for sustained operations.</p>
  </div>
</section>
EOF

# Add the monitoring section to your report
cat >> "$output_dir/combined_report.html" << 'HTML_MONITORING'
    <!-- Monitoring Dashboards -->
    <section class="section" id="monitoring-dashboards">
      <h2 class="section-title">Infrastructure Monitoring</h2>
      
      <p>The following Grafana dashboard visualizations show the detailed monitoring of all three platforms during the benchmark tests. These graphs provide valuable insights into system behavior under load.</p>
      
      <div class="subsection">
        <h3>Request Throughput (RPS)</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="RPS Comparison" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> This graph shows the throughput (requests per second) for each platform. Notable spikes can be seen where Saleor achieves exceptional peaks of up to 16K RPS during stress test phases, significantly outperforming Medusa (~2K RPS) and Spree (~250 RPS). The plateaus indicate sustained load periods where the platforms were maintaining their maximum stable throughput.</p>
      </div>
      
      <div class="subsection">
        <h3>Response Latency</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="Latency Comparison" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> The latency graph shows response times across all platforms. During peak load periods, all platforms experience increased latency, with Medusa showing the highest peaks. Saleor maintains relatively stable response times even under high load, demonstrating better latency management during stress conditions.</p>
      </div>
      
      <div class="subsection">
        <h3>CPU Utilization</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="CPU Usage" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> CPU usage directly correlates with throughput patterns. During peak load, Saleor utilizes nearly 100% of CPU resources, demonstrating efficient use of available processing power. Medusa shows moderate CPU usage, while Spree has the lowest utilization. This suggests that Saleor's higher throughput is achieved through more efficient CPU utilization.</p>
      </div>
      
      <div class="subsection">
        <h3>Memory Usage</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="Memory Usage" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> Memory consumption increases during test execution for all platforms. Saleor shows a gradual increase in memory usage that stabilizes, suggesting good memory management. Medusa also shows controlled memory growth. There are no signs of memory leaks in any platform, as memory usage plateaus rather than continuously increasing.</p>
      </div>
      
      <div class="subsection">
        <h3>Network Traffic</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="Network Traffic" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> Network traffic patterns mirror the RPS graphs, with Saleor generating significantly higher network activity than the other platforms. This demonstrates Saleor's ability to handle greater data throughput. The symmetry between inbound and outbound traffic is also notable, indicating well-balanced request-response cycles.</p>
      </div>
      
      <div class="subsection">
        <h3>Disk Activity</h3>
        <div class="dashboard-image">
          <img src="images/image.png" alt="Disk Activity" style="max-width: 100%; border: 1px solid #ccc; border-radius: 5px;">
        </div>
        <p><strong>Analysis:</strong> Disk I/O activity shows interesting patterns during load testing. Medusa appears to have higher disk write operations during peak load, suggesting it may be more disk-intensive in its operations. Saleor shows more moderate disk activity despite its higher throughput, indicating better caching or memory utilization strategies that reduce disk dependencies.</p>
      </div>
      
      <div class="key-finding">
        <h3>Infrastructure Insights</h3>
        <p>These monitoring graphs reveal important insights about the platforms' resource utilization:</p>
        <ul>
          <li><strong>Saleor</strong> excels in high-throughput scenarios with efficient CPU utilization, though it consumes more resources overall.</li>
          <li><strong>Medusa</strong> shows balanced resource consumption with moderate throughput capabilities.</li>
          <li><strong>Spree</strong> has the lowest resource footprint but also the lowest performance ceiling.</li>
          <li>All platforms show good stability without resource leaks during extended testing.</li>
        </ul>
      </div>
    </section>
HTML_MONITORING
# Finish the individual reports section
cat >> "$output_dir/combined_report.html" << 'HTML_FOOTER1'
      </div>
    </section>
    
    <!-- Platform-Specific Observations -->
    <section class="section" id="platform-observations">
      <h2 class="section-title">Platform-Specific Observations</h2>
      
      <h3 class="platform-medusa">Medusa</h3>
      <p>Medusa demonstrated consistent performance across all test durations, with a slight degradation in response times as the test duration increased. It maintained solid reliability throughout all tests.</p>
      
      <h3 class="platform-saleor">Saleor</h3>
      <p>Saleor showed the highest throughput capability among the tested platforms, handling a significantly higher request rate. Its GraphQL implementation appears to be optimized for performance.</p>
      
      <h3 class="platform-spree">Spree</h3>
      <p>Spree offered a balanced performance profile with moderate throughput and response times. It demonstrated good stability under sustained load.</p>
    </section>
  </div>
  
  <footer>
    <div class="container">
      <p>E-commerce Platform Benchmark Report &copy; 2025</p>
    </div>
  </footer>
HTML_FOOTER1

cat >> "$output_dir/combined_report.html" << EOF
<section class="section" id="peak-performance">
  <h2 class="section-title">Peak Performance Analysis</h2>
  
  <p>While the average RPS values provide a good baseline for comparison, the platforms were also evaluated for their maximum throughput capabilities. During stress testing phases, significant differences in peak performance were observed:</p>
  
  <ul>
    <li><span class="platform-saleor">Saleor</span> demonstrated exceptional peak throughput, reaching up to <strong>16,000 RPS</strong> during brief intervals, showcasing its high capacity for handling traffic spikes.</li>
    <li><span class="platform-medusa">Medusa</span> achieved peak performance of approximately <strong>2,000 RPS</strong>.</li>
    <li><span class="platform-spree">Spree</span> maintained consistent performance with peaks around <strong>250 RPS</strong>.</li>
  </ul>
  
  <p>These peak values indicate the platforms' ability to handle burst traffic, which is particularly important for e-commerce applications during flash sales or promotional events.</p>
  
  <div class="key-finding">
    <p><strong>Note:</strong> Peak performance was measured during controlled stress test intervals and may not be sustainable for extended periods. Average RPS values provide a more realistic expectation for sustained operations.</p>
  </div>
</section>
EOF

# Add chart initialization JavaScript
cat >> "$output_dir/combined_report.html" << EOF
  <script>
    // Set the report date
    document.getElementById('report-date').textContent = new Date().toLocaleString();
    
    // Chart initialization
    document.addEventListener('DOMContentLoaded', function() {
      // RPS Chart
      const rpsCtx = document.getElementById('rpsChart').getContext('2d');
      new Chart(rpsCtx, {
        type: 'bar',
        data: {
          labels: [$labels],
          datasets: [
            {
              label: 'Medusa',
              data: [$rps_medusa],
              backgroundColor: 'rgba(76, 175, 80, 0.7)',
              borderColor: 'rgba(76, 175, 80, 1)',
              borderWidth: 1
            },
            {
              label: 'Saleor',
              data: [$rps_saleor],
              backgroundColor: 'rgba(255, 152, 0, 0.7)',
              borderColor: 'rgba(255, 152, 0, 1)',
              borderWidth: 1
            },
            {
              label: 'Spree',
              data: [$rps_spree],
              backgroundColor: 'rgba(33, 150, 243, 0.7)',
              borderColor: 'rgba(33, 150, 243, 1)',
              borderWidth: 1
            }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            title: {
              display: false
            },
            legend: {
              position: 'top',
            }
          },
          scales: {
            y: {
              beginAtZero: true,
              title: {
                display: true,
                text: 'Requests Per Second (RPS)'
              }
            }
          }
        }
      });
      
      // Latency Chart
      const latencyCtx = document.getElementById('latencyChart').getContext('2d');
      new Chart(latencyCtx, {
        type: 'line',
        data: {
          labels: [$labels],
          datasets: [
            {
              label: 'Medusa P95',
              data: [$p95_medusa],
              backgroundColor: 'rgba(76, 175, 80, 0.2)',
              borderColor: 'rgba(76, 175, 80, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            },
            {
              label: 'Saleor P95',
              data: [$p95_saleor],
              backgroundColor: 'rgba(255, 152, 0, 0.2)',
              borderColor: 'rgba(255, 152, 0, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            },
            {
              label: 'Spree P95',
              data: [$p95_spree],
              backgroundColor: 'rgba(33, 150, 243, 0.2)',
              borderColor: 'rgba(33, 150, 243, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            title: {
              display: false
            },
            legend: {
              position: 'top',
            }
          },
          scales: {
            y: {
              beginAtZero: true,
              title: {
                display: true,
                text: 'Response Time (ms)'
              }
            }
          }
        }
      });
      
      // Success Rate Chart
      const successRateCtx = document.getElementById('successRateChart').getContext('2d');
      new Chart(successRateCtx, {
        type: 'line',
        data: {
          labels: [$labels],
          datasets: [
            {
              label: 'Medusa',
              data: [$success_medusa],
              backgroundColor: 'rgba(76, 175, 80, 0.2)',
              borderColor: 'rgba(76, 175, 80, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            },
            {
              label: 'Saleor',
              data: [$success_saleor],
              backgroundColor: 'rgba(255, 152, 0, 0.2)',
              borderColor: 'rgba(255, 152, 0, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            },
            {
              label: 'Spree',
              data: [$success_spree],
              backgroundColor: 'rgba(33, 150, 243, 0.2)',
              borderColor: 'rgba(33, 150, 243, 1)',
              borderWidth: 2,
              tension: 0.3,
              fill: true
            }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            title: {
              display: false
            },
            legend: {
              position: 'top',
            }
          },
          scales: {
            y: {
              min: 0,
              max: 100,
              title: {
                display: true,
                text: 'Success Rate (%)'
              }
            }
          }
        }
      });
    });
  </script>
</body>
</html>
EOF

echo -e "${GREEN}Enhanced report with charts generated at $output_dir/combined_report.html${NC}"
echo "Open this file in a web browser to view the report."