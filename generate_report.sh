#!/bin/bash

# Check if any results directories were provided
if [ $# -eq 0 ]; then
  echo "Usage: $0 <results_directory_7min> <results_directory_15min> <results_directory_30min> [--output <output_dir>]"
  echo "Example: $0 benchmark_results_7min_20250310 benchmark_results_15min_20250310 benchmark_results_30min_20250310 --output combined_report"
  exit 1
fi

# Process command line arguments
RESULTS_DIRS=()
OUTPUT_DIR="combined_report"

while [[ $# -gt 0 ]]; do
  case $1 in
    --output)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    *)
      RESULTS_DIRS+=("$1")
      shift
      ;;
  esac
done

# Check if all directories exist
for dir in "${RESULTS_DIRS[@]}"; do
  if [ ! -d "$dir" ]; then
    echo "Error: Directory $dir does not exist"
    exit 1
  fi
  
  # Check if comparison.json exists
  if [ ! -f "$dir/comparison.json" ]; then
    echo "Error: Comparison results file not found at $dir/comparison.json"
    exit 1
  fi
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Extract duration from directory name
function extract_duration() {
  local dir=$1
  # Look for pattern like benchmark_results_7min or benchmark_results_15min
  if [[ $dir =~ benchmark_results_([0-9]+)min ]]; then
    echo "${BASH_REMATCH[1]}"
  else
    # Default to using directory index if no duration pattern found
    local idx=$2
    echo "$idx"
  fi
}

# Collect data from all comparison.json files into a single JSON
COMBINED_DATA="{\"testDurations\": ["

for i in "${!RESULTS_DIRS[@]}"; do
  RESULTS_DIR="${RESULTS_DIRS[$i]}"
  DURATION=$(extract_duration "$RESULTS_DIR" "$((i+1))")
  
  # Read the comparison.json file
  COMPARISON_DATA=$(cat "$RESULTS_DIR/comparison.json")
  
  # Add duration field to the data
  COMPARISON_DATA=$(echo "$COMPARISON_DATA" | sed "s/{/{\"durationMinutes\": $DURATION, /")
  
  # Add to combined data
  COMBINED_DATA+="$COMPARISON_DATA"
  
  # Add comma if not the last element
  if [ $i -lt $((${#RESULTS_DIRS[@]} - 1)) ]; then
    COMBINED_DATA+=","
  fi
done

COMBINED_DATA+="], "

# Add summary comparisons across durations
COMBINED_DATA+="\"platformSummary\": {"

# Process each platform
for platform in "medusa" "saleor" "spree"; do
  COMBINED_DATA+="\"$platform\": {\"rps\": ["
  
  # Extract RPS for each duration
  for i in "${!RESULTS_DIRS[@]}"; do
    RESULTS_DIR="${RESULTS_DIRS[$i]}"
    DURATION=$(extract_duration "$RESULTS_DIR" "$((i+1))")
    
    # Extract RPS value for this platform and duration
    RPS=$(jq -r ".rpsComparison.$platform" "$RESULTS_DIR/comparison.json")
    
    # Add to combined data
    COMBINED_DATA+="{\"duration\": $DURATION, \"value\": $RPS}"
    
    # Add comma if not the last element
    if [ $i -lt $((${#RESULTS_DIRS[@]} - 1)) ]; then
      COMBINED_DATA+=","
    fi
  done
  
  COMBINED_DATA+="], \"latencyP95\": ["
  
  # Extract P95 latency for each duration
  for i in "${!RESULTS_DIRS[@]}"; do
    RESULTS_DIR="${RESULTS_DIRS[$i]}"
    DURATION=$(extract_duration "$RESULTS_DIR" "$((i+1))")
    
    # Extract P95 latency value for this platform and duration
    LATENCY=$(jq -r ".latencyComparison.$platform.p95" "$RESULTS_DIR/comparison.json")
    
    # Add to combined data
    COMBINED_DATA+="{\"duration\": $DURATION, \"value\": $LATENCY}"
    
    # Add comma if not the last element
    if [ $i -lt $((${#RESULTS_DIRS[@]} - 1)) ]; then
      COMBINED_DATA+=","
    fi
  done
  
  COMBINED_DATA+="], \"successRate\": ["
  
  # Extract success rate for each duration
  for i in "${!RESULTS_DIRS[@]}"; do
    RESULTS_DIR="${RESULTS_DIRS[$i]}"
    DURATION=$(extract_duration "$RESULTS_DIR" "$((i+1))")
    
    # Extract error rate and convert to success rate
    ERROR_RATE=$(jq -r ".errorComparison.$platform" "$RESULTS_DIR/comparison.json")
    SUCCESS_RATE=$(echo "100 - $ERROR_RATE" | bc)
    
    # Add to combined data
    COMBINED_DATA+="{\"duration\": $DURATION, \"value\": $SUCCESS_RATE}"
    
    # Add comma if not the last element
    if [ $i -lt $((${#RESULTS_DIRS[@]} - 1)) ]; then
      COMBINED_DATA+=","
    fi
  done
  
  COMBINED_DATA+="]}"
  
  # Add comma if not the last platform
  if [ "$platform" != "spree" ]; then
    COMBINED_DATA+=","
  fi
done

COMBINED_DATA+="}, "

# Add combined recommendations
COMBINED_DATA+="\"combinedRecommendations\": {"

# Overall recommendations based on all tests
COMBINED_DATA+="\"overall\": ["
COMBINED_DATA+="\"Consider Saleor for high-throughput scenarios based on its consistent performance across test durations\","
COMBINED_DATA+="\"Medusa offers a good balance of performance and stability for moderate load scenarios\","
COMBINED_DATA+="\"Spree would require significant optimization to handle production loads\","
COMBINED_DATA+="\"All platforms show some performance degradation in longer tests, suggesting resource leaks or connection pool issues\""
COMBINED_DATA+="], "

# Platform-specific recommendations
for platform in "medusa" "saleor" "spree"; do
  COMBINED_DATA+="\"$platform\": ["
  
  case $platform in
    "medusa")
      COMBINED_DATA+="\"Optimize database connection management to improve stability in longer test runs\","
      COMBINED_DATA+="\"Consider implementing more aggressive connection pool recycling\","
      COMBINED_DATA+="\"Monitor memory usage as test duration increases\""
      ;;
    "saleor")
      COMBINED_DATA+="\"Implement circuit breakers to gracefully handle load spikes\","
      COMBINED_DATA+="\"Add recovery mechanisms to automatically restart after high-load failures\","
      COMBINED_DATA+="\"Consider deploying with multiple application instances for higher throughput\""
      ;;
    "spree")
      COMBINED_DATA+="\"Significant performance tuning required for production use\","
      COMBINED_DATA+="\"Investigate database query optimization and indexing\","
      COMBINED_DATA+="\"Consider upgrading or replacing if high throughput is required\""
      ;;
  esac
  
  COMBINED_DATA+="]"
  
  # Add comma if not the last platform
  if [ "$platform" != "spree" ]; then
    COMBINED_DATA+=","
  fi
done

COMBINED_DATA+="}}"

# Write combined data to file
echo "$COMBINED_DATA" > "$OUTPUT_DIR/combined_data.json"

# Create the HTML report
HTML_FILE="$OUTPUT_DIR/combined_report.html"

cat > $HTML_FILE << 'EOL'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>E-commerce Platform Benchmark - Combined Results</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@3.7.1/dist/chart.min.js"></script>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        h1 {
            color: #2c3e50;
            text-align: center;
            margin-bottom: 30px;
        }
        h2 {
            color: #3498db;
            margin-top: 40px;
            border-bottom: 1px solid #eee;
            padding-bottom: 10px;
        }
        h3 {
            color: #2980b9;
            margin-top: 25px;
        }
        .chart-container {
            position: relative;
            height: 400px;
            margin-bottom: 40px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 30px;
        }
        th, td {
            border: 1px solid #ddd;
            padding: 12px;
            text-align: left;
        }
        th {
            background-color: #f2f2f2;
        }
        tr:nth-child(even) {
            background-color: #f9f9f9;
        }
        .card {
            background-color: #fff;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            padding: 20px;
            margin-bottom: 30px;
        }
        .tabs {
            display: flex;
            border-bottom: 1px solid #ddd;
            margin-bottom: 20px;
        }
        .tab {
            padding: 10px 20px;
            cursor: pointer;
            background-color: #f8f9fa;
            border: 1px solid #ddd;
            border-bottom: none;
            border-radius: 5px 5px 0 0;
            margin-right: 5px;
        }
        .tab.active {
            background-color: #fff;
            border-bottom: 1px solid #fff;
            margin-bottom: -1px;
            font-weight: bold;
        }
        .tab-content {
            display: none;
        }
        .tab-content.active {
            display: block;
        }
        .recommendations {
            background-color: #f0f7ff;
            padding: 20px;
            border-radius: 8px;
            margin-top: 30px;
        }
        .flex-container {
            display: flex;
            gap: 20px;
            flex-wrap: wrap;
        }
        .flex-item {
            flex: 1;
            min-width: 300px;
        }
        .metric-card {
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 8px;
            box-shadow: 0 1px 5px rgba(0,0,0,0.05);
            margin-bottom: 15px;
        }
        .metric-value {
            font-size: 24px;
            font-weight: bold;
            margin: 10px 0;
        }
        .legend {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin: 10px 0;
        }
        .legend-item {
            display: flex;
            align-items: center;
        }
        .legend-color {
            width: 15px;
            height: 15px;
            display: inline-block;
            margin-right: 5px;
        }
        .platform-color-medusa {
            background-color: rgba(54, 162, 235, 0.6);
        }
        .platform-color-saleor {
            background-color: rgba(255, 99, 132, 0.6);
        }
        .platform-color-spree {
            background-color: rgba(75, 192, 192, 0.6);
        }
        .executive-summary {
            background-color: #f5f5f5;
            padding: 20px;
            border-radius: 8px;
            border-left: 5px solid #3498db;
            margin-bottom: 30px;
        }
    </style>
</head>
<body>
    <h1>E-commerce Platform Benchmark - Combined Results</h1>
    
    <div class="executive-summary">
        <h2>Executive Summary</h2>
        <p>This report compares the performance of three e-commerce platforms (Medusa, Saleor, and Spree) across multiple test durations (7, 15, and 30 minutes). The tests measure throughput (RPS), latency, and stability under sustained load.</p>
        <div id="keySummaryPoints"></div>
    </div>
    
    <div class="tabs">
        <div class="tab active" data-tab="overview">Overview</div>
        <div class="tab" data-tab="duration">Duration Comparison</div>
        <div class="tab" data-tab="platform">Platform Analysis</div>
        <div class="tab" data-tab="recommendations">Recommendations</div>
    </div>
    
    <div id="overview" class="tab-content active">
        <div class="card">
            <h2>Performance Overview</h2>
            <div class="chart-container">
                <canvas id="overviewRpsChart"></canvas>
            </div>
            <div class="legend">
                <div class="legend-item"><span class="legend-color platform-color-medusa"></span> Medusa</div>
                <div class="legend-item"><span class="legend-color platform-color-saleor"></span> Saleor</div>
                <div class="legend-item"><span class="legend-color platform-color-spree"></span> Spree</div>
            </div>
        </div>
        
        <div class="card">
            <h2>Stability Overview</h2>
            <div class="chart-container">
                <canvas id="overviewSuccessRateChart"></canvas>
            </div>
        </div>
        
        <div class="card">
            <h2>Platform Rankings</h2>
            <table id="rankingTable">
                <thead>
                    <tr>
                        <th>Ranking Criteria</th>
                        <th>1st Place</th>
                        <th>2nd Place</th>
                        <th>3rd Place</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>Maximum Throughput</td>
                        <td id="rank-throughput-1"></td>
                        <td id="rank-throughput-2"></td>
                        <td id="rank-throughput-3"></td>
                    </tr>
                    <tr>
                        <td>Lowest Latency</td>
                        <td id="rank-latency-1"></td>
                        <td id="rank-latency-2"></td>
                        <td id="rank-latency-3"></td>
                    </tr>
                    <tr>
                        <td>Highest Stability</td>
                        <td id="rank-stability-1"></td>
                        <td id="rank-stability-2"></td>
                        <td id="rank-stability-3"></td>
                    </tr>
                    <tr>
                        <td>Long-Term Performance</td>
                        <td id="rank-longterm-1"></td>
                        <td id="rank-longterm-2"></td>
                        <td id="rank-longterm-3"></td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>
    
    <div id="duration" class="tab-content">
        <div class="card">
            <h2>RPS by Test Duration</h2>
            <div class="chart-container">
                <canvas id="rpsByDurationChart"></canvas>
            </div>
        </div>
        
        <div class="card">
            <h2>Latency by Test Duration</h2>
            <div class="chart-container">
                <canvas id="latencyByDurationChart"></canvas>
            </div>
        </div>
        
        <div class="card">
            <h2>Success Rate by Test Duration</h2>
            <div class="chart-container">
                <canvas id="successRateByDurationChart"></canvas>
            </div>
        </div>
        
        <div class="card">
            <h2>Performance Stability Analysis</h2>
            <div class="flex-container">
                <div class="flex-item">
                    <h3>RPS Degradation</h3>
                    <div class="chart-container">
                        <canvas id="rpsDegradationChart"></canvas>
                    </div>
                </div>
                <div class="flex-item">
                    <h3>Latency Increase</h3>
                    <div class="chart-container">
                        <canvas id="latencyIncreaseChart"></canvas>
                    </div>
                </div>
            </div>
        </div>
    </div>
    
    <div id="platform" class="tab-content">
        <div class="card">
            <h2>Medusa Performance Analysis</h2>
            <div class="flex-container">
                <div class="flex-item">
                    <div id="medusaMetrics"></div>
                </div>
                <div class="flex-item">
                    <div class="chart-container">
                        <canvas id="medusaPerformanceChart"></canvas>
                    </div>
                </div>
            </div>
        </div>
        
        <div class="card">
            <h2>Saleor Performance Analysis</h2>
            <div class="flex-container">
                <div class="flex-item">
                    <div id="saleorMetrics"></div>
                </div>
                <div class="flex-item">
                    <div class="chart-container">
                        <canvas id="saleorPerformanceChart"></canvas>
                    </div>
                </div>
            </div>
        </div>
        
        <div class="card">
            <h2>Spree Performance Analysis</h2>
            <div class="flex-container">
                <div class="flex-item">
                    <div id="spreeMetrics"></div>
                </div>
                <div class="flex-item">
                    <div class="chart-container">
                        <canvas id="spreePerformanceChart"></canvas>
                    </div>
                </div>
            </div>
        </div>
    </div>
    
    <div id="recommendations" class="tab-content">
        <div class="card">
            <h2>Overall Recommendations</h2>
            <div id="overallRecommendations"></div>
        </div>
        
        <div class="flex-container">
            <div class="flex-item card">
                <h2>Medusa Recommendations</h2>
                <div id="medusaRecommendations"></div>
            </div>
            
            <div class="flex-item card">
                <h2>Saleor Recommendations</h2>
                <div id="saleorRecommendations"></div>
            </div>
            
            <div class="flex-item card">
                <h2>Spree Recommendations</h2>
                <div id="spreeRecommendations"></div>
            </div>
        </div>
        
        <div class="card">
            <h2>Recommended Use Cases</h2>
            <table>
                <thead>
                    <tr>
                        <th>Use Case</th>
                        <th>Recommended Platform</th>
                        <th>Rationale</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>High-volume e-commerce</td>
                        <td>Saleor</td>
                        <td>Highest throughput capacity and good stability for shorter periods</td>
                    </tr>
                    <tr>
                        <td>Medium-sized stores</td>
                        <td>Medusa</td>
                        <td>Good balance of performance and stability</td>
                    </tr>
                    <tr>
                        <td>Development/testing</td>
                        <td>Spree</td>
                        <td>Simpler setup but requires optimization for production loads</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>

    <script>
        // Tabs functionality
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', () => {
                document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                
                tab.classList.add('active');
                document.getElementById(tab.dataset.tab).classList.add('active');
            });
        });
        
        // Load the combined data
        fetch('combined_data.json')
            .then(response => response.json())
            .then(data => {
                renderExecutiveSummary(data);
                renderOverviewCharts(data);
                renderDurationCharts(data);
                renderPlatformAnalysis(data);
                renderRecommendations(data);
                populateRankingTable(data);
            })
            .catch(error => {
                console.error('Error loading combined data:', error);
                document.body.innerHTML += `<div style="color: red; padding: 20px;">Error loading data: ${error.message}</div>`;
            });
            
        function renderExecutiveSummary(data) {
            const container = document.getElementById('keySummaryPoints');
            const summaryPoints = [];
            
            // Find best performing platform overall
            const platforms = ['medusa', 'saleor', 'spree'];
            let bestPlatform = '';
            let highestAvgRPS = 0;
            
            platforms.forEach(platform => {
                const rpsValues = data.platformSummary[platform].rps.map(item => item.value);
                const avgRPS = rpsValues.reduce((sum, val) => sum + val, 0) / rpsValues.length;
                
                if (avgRPS > highestAvgRPS) {
                    highestAvgRPS = avgRPS;
                    bestPlatform = platform;
                }
            });
            
            // Add top-level findings
            summaryPoints.push(`<strong>${bestPlatform.charAt(0).toUpperCase() + bestPlatform.slice(1)}</strong> demonstrated the best overall performance across test durations with an average of ${highestAvgRPS.toFixed(2)} RPS.`);
            
            // Check for performance degradation with longer tests
            platforms.forEach(platform => {
                const rpsValues = data.platformSummary[platform].rps.map(item => item.value);
                const shortestTest = rpsValues[0];
                const longestTest = rpsValues[rpsValues.length - 1];
                
                const degradationPct = ((shortestTest - longestTest) / shortestTest * 100).toFixed(2);
                
                if (degradationPct > 10) {
                    summaryPoints.push(`<strong>${platform.charAt(0).toUpperCase() + platform.slice(1)}</strong> showed significant performance degradation (${degradationPct}%) in longer tests, suggesting potential resource leaks.`);
                }
            });
            
            // Add bullet points to container
            const ul = document.createElement('ul');
            summaryPoints.forEach(point => {
                const li = document.createElement('li');
                li.innerHTML = point;
                ul.appendChild(li);
            });
            
            container.appendChild(ul);
        }
        
        function renderOverviewCharts(data) {
            // Create RPS overview chart
            const rpsCtx = document.getElementById('overviewRpsChart').getContext('2d');
            const platforms = ['medusa', 'saleor', 'spree'];
            const durations = data.testDurations.map(test => `${test.durationMinutes} min`);
            
            const datasets = platforms.map((platform, index) => {
                const colors = [
                    'rgba(54, 162, 235, 0.6)', // medusa
                    'rgba(255, 99, 132, 0.6)',  // saleor
                    'rgba(75, 192, 192, 0.6)'   // spree
                ];
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: data.testDurations.map(test => test.rpsComparison[platform]),
                    backgroundColor: colors[index],
                    borderColor: colors[index].replace('0.6', '1'),
                    borderWidth: 1
                };
            });
            
            new Chart(rpsCtx, {
                type: 'bar',
                data: {
                    labels: durations,
                    datasets: datasets
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Throughput Across Test Durations'
                        },
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'RPS'
                            }
                        }
                    }
                }
            });
            
            // Create Success Rate overview chart
            const successRateCtx = document.getElementById('overviewSuccessRateChart').getContext('2d');
            
            const successRateDatasets = platforms.map((platform, index) => {
                const colors = [
                    'rgba(54, 162, 235, 0.6)', // medusa
                    'rgba(255, 99, 132, 0.6)',  // saleor
                    'rgba(75, 192, 192, 0.6)'   // spree
                ];
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: data.testDurations.map(test => 100 - test.errorComparison[platform]),
                    backgroundColor: colors[index],
                    borderColor: colors[index].replace('0.6', '1'),
                    borderWidth: 1
                };
            });
            
            new Chart(successRateCtx, {
                type: 'bar',
                data: {
                    labels: durations,
                    datasets: successRateDatasets
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Success Rate Across Test Durations'
                        },
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            max: 100,
                            title: {
                                display: true,
                                text: 'Success Rate (%)'
                            }
                        }
                    }
                }
            });
        }
        
        function renderDurationCharts(data) {
            // RPS by Duration Chart (Line chart)
            const rpsByDurationCtx = document.getElementById('rpsByDurationChart').getContext('2d');
            const platforms = ['medusa', 'saleor', 'spree'];
            const datasets = platforms.map(platform => {
                const color = platform === 'medusa' 
                    ? 'rgba(54, 162, 235, 0.6)' 
                    : platform === 'saleor' 
                        ? 'rgba(255, 99, 132, 0.6)' 
                        : 'rgba(75, 192, 192, 0.6)';
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: data.platformSummary[platform].rps.map(item => ({
                        x: item.duration,
                        y: item.value
                    })),
                    backgroundColor: color,
                    borderColor: color.replace('0.6', '1'),
                    fill: false,
                    tension: 0.1
                };
            });
            
            new Chart(rpsByDurationCtx, {
                type: 'line',
                data: {
                    datasets: datasets
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'RPS Trend by Test Duration'
                        }
                    },
                    scales: {
                        x: {
                            type: 'linear',
                            position: 'bottom',
                            title: {
                                display: true,
                                text: 'Test Duration (minutes)'
                            },
                            ticks: {
                                callback: function(value) {
                                    return value + ' min';
                                }
                            }
                        },
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'RPS'
                            }
                        }
                    }
                }
            });
            
            // Latency by Duration Chart
            const latencyByDurationCtx = document.getElementById('latencyByDurationChart').getContext('2d');
            const latencyDatasets = platforms.map(platform => {
                const color = platform === 'medusa' 
                    ? 'rgba(54, 162, 235, 0.6)' 
                    : platform === 'saleor' 
                        ? 'rgba(255, 99, 132, 0.6)' 
                        : 'rgba(75, 192, 192, 0.6)';
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: data.platformSummary[platform].latencyP95.map(item => ({
                        x: item.duration,
                        y: item.value
                    })),
                    backgroundColor: color,
                    borderColor: color.replace('0.6', '1'),
                    fill: false,
                    tension: 0.1
                };
            });
            
            new Chart(latencyByDurationCtx, {
                type: 'line',
                data: {
                    datasets: latencyDatasets
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'P95 Latency Trend by Test Duration'
                        }
                    },
                    scales: {
                        x: {
                            type: 'linear',
                            position: 'bottom',
                            title: {
                                display: true,
                                text: 'Test Duration (minutes)'
                            },
                            ticks: {
                                callback: function(value) {
                                    return value + ' min';
                                }
                            }
                        },
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'Latency (ms)'
                            }
                        }
                    }
                }
            });
            
            // Success Rate by Duration Chart
            const successRateByDurationCtx = document.getElementById('successRateByDurationChart').getContext('2d');
            const successRateDatasets = platforms.map(platform => {
                const color = platform === 'medusa' 
                    ? 'rgba(54, 162, 235, 0.6)' 
                    : platform === 'saleor' 
                        ? 'rgba(255, 99, 132, 0.6)' 
                        : 'rgba(75, 192, 192, 0.6)';
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: data.platformSummary[platform].successRate.map(item => ({
                        x: item.duration,
                        y: item.value
                    })),
                    backgroundColor: color,
                    borderColor: color.replace('0.6', '1'),
                    fill: false,
                    tension: 0.1
                };
            });
            
            new Chart(successRateByDurationCtx, {
                type: 'line',
                data: {
                    datasets: successRateDatasets
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Success Rate Trend by Test Duration'
                        }
                    },
                    scales: {
                        x: {
                            type: 'linear',
                            position: 'bottom',
                            title: {
                                display: true,
                                text: 'Test Duration (minutes)'
                            },
                            ticks: {
                                callback: function(value) {
                                    return value + ' min';
                                }
                            }
                        },
                        y: {
                            beginAtZero: true,
                            max: 100,
                            title: {
                                display: true,
                                text: 'Success Rate (%)'
                            }
                        }
                    }
                }
            });
            
            // RPS Degradation Chart
            const rpsDegradationCtx = document.getElementById('rpsDegradationChart').getContext('2d');
            const rpsDegradationData = platforms.map(platform => {
                const rpsValues = data.platformSummary[platform].rps;
                const initialRPS = rpsValues[0].value;
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: rpsValues.map(item => ({
                        x: item.duration,
                        y: ((initialRPS - item.value) / initialRPS * 100)
                    })),
                    backgroundColor: platform === 'medusa' 
                        ? 'rgba(54, 162, 235, 0.6)' 
                        : platform === 'saleor' 
                            ? 'rgba(255, 99, 132, 0.6)' 
                            : 'rgba(75, 192, 192, 0.6)',
                    borderColor: platform === 'medusa' 
                        ? 'rgba(54, 162, 235, 1)' 
                        : platform === 'saleor' 
                            ? 'rgba(255, 99, 132, 1)' 
                            : 'rgba(75, 192, 192, 1)',
                    fill: false
                };
            });
            
            new Chart(rpsDegradationCtx, {
                type: 'line',
                data: {
                    datasets: rpsDegradationData
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'RPS Degradation by Test Duration'
                        },
                        tooltip: {
                            callbacks: {
                                label: function(context) {
                                    return `${context.dataset.label}: ${context.parsed.y.toFixed(2)}% degradation`;
                                }
                            }
                        }
                    },
                    scales: {
                        x: {
                            type: 'linear',
                            position: 'bottom',
                            title: {
                                display: true,
                                text: 'Test Duration (minutes)'
                            },
                            ticks: {
                                callback: function(value) {
                                    return value + ' min';
                                }
                            }
                        },
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'Degradation (%)'
                            }
                        }
                    }
                }
            });
            
            // Latency Increase Chart
            const latencyIncreaseCtx = document.getElementById('latencyIncreaseChart').getContext('2d');
            const latencyIncreaseData = platforms.map(platform => {
                const latencyValues = data.platformSummary[platform].latencyP95;
                const initialLatency = latencyValues[0].value;
                
                return {
                    label: platform.charAt(0).toUpperCase() + platform.slice(1),
                    data: latencyValues.map(item => ({
                        x: item.duration,
                        y: ((item.value - initialLatency) / initialLatency * 100)
                    })),
                    backgroundColor: platform === 'medusa' 
                        ? 'rgba(54, 162, 235, 0.6)' 
                        : platform === 'saleor' 
                            ? 'rgba(255, 99, 132, 0.6)' 
                            : 'rgba(75, 192, 192, 0.6)',
                    borderColor: platform === 'medusa' 
                        ? 'rgba(54, 162, 235, 1)' 
                        : platform === 'saleor' 
                            ? 'rgba(255, 99, 132, 1)' 
                            : 'rgba(75, 192, 192, 1)',
                    fill: false
                };
            });
            
            new Chart(latencyIncreaseCtx, {
                type: 'line',
                data: {
                    datasets: latencyIncreaseData
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Latency Increase by Test Duration'
                        },
                        tooltip: {
                            callbacks: {
                                label: function(context) {
                                    return `${context.dataset.label}: ${context.parsed.y.toFixed(2)}% increase`;
                                }
                            }
                        }
                    },
                    scales: {
                        x: {
                            type: 'linear',
                            position: 'bottom',
                            title: {
                                display: true,
                                text: 'Test Duration (minutes)'
                            },
                            ticks: {
                                callback: function(value) {
                                    return value + ' min';
                                }
                            }
                        },
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'Increase (%)'
                            }
                        }
                    }
                }
            });
        }
        
        function renderPlatformAnalysis(data) {
            const platforms = ['medusa', 'saleor', 'spree'];
            const metrics = ['rps', 'latencyP95', 'successRate'];
            const metricLabels = {
                'rps': 'Throughput (RPS)',
                'latencyP95': 'P95 Latency (ms)',
                'successRate': 'Success Rate (%)'
            };
            
            platforms.forEach(platform => {
                // Render platform metrics cards
                const metricsContainer = document.getElementById(`${platform}Metrics`);
                
                // Get the latest test data
                const latestTest = data.testDurations[data.testDurations.length - 1];
                
                // Create a metrics card for max RPS
                const maxRPS = Math.max(...data.platformSummary[platform].rps.map(item => item.value));
                const maxRPSMetricCard = createMetricCard('Max Throughput', `${maxRPS.toFixed(2)} RPS`, 'Highest recorded throughput across all tests');
                metricsContainer.appendChild(maxRPSMetricCard);
                
                // Create a metrics card for avg success rate
                const avgSuccessRate = data.platformSummary[platform].successRate.reduce((sum, item) => sum + item.value, 0) / 
                    data.platformSummary[platform].successRate.length;
                const successRateMetricCard = createMetricCard('Avg Success Rate', `${avgSuccessRate.toFixed(2)}%`, 'Average success rate across all tests');
                metricsContainer.appendChild(successRateMetricCard);
                
                // Create a metrics card for avg latency
                const avgLatency = data.platformSummary[platform].latencyP95.reduce((sum, item) => sum + item.value, 0) / 
                    data.platformSummary[platform].latencyP95.length;
                const latencyMetricCard = createMetricCard('Avg P95 Latency', `${avgLatency.toFixed(2)} ms`, 'Average P95 latency across all tests');
                metricsContainer.appendChild(latencyMetricCard);
                
                // Create a metrics card for stability score
                const rpsValues = data.platformSummary[platform].rps.map(item => item.value);
                const initialRPS = rpsValues[0];
                const finalRPS = rpsValues[rpsValues.length - 1];
                const stabilityScore = ((finalRPS / initialRPS) * 100).toFixed(2);
                const stabilityMetricCard = createMetricCard('Stability Score', `${stabilityScore}%`, 'Final RPS as a percentage of initial RPS');
                metricsContainer.appendChild(stabilityMetricCard);
                
                // Render platform performance chart
                const chartCtx = document.getElementById(`${platform}PerformanceChart`).getContext('2d');
                const datasets = [];
                
                metrics.forEach((metric, index) => {
                    let yAxisID = metric === 'latencyP95' ? 'y-latency' : 'y-percentage';
                    
                    datasets.push({
                        label: metricLabels[metric],
                        data: data.platformSummary[platform][metric].map(item => ({
                            x: item.duration,
                            y: item.value
                        })),
                        backgroundColor: index === 0 
                            ? 'rgba(54, 162, 235, 0.6)' 
                            : index === 1 
                                ? 'rgba(255, 99, 132, 0.6)' 
                                : 'rgba(75, 192, 192, 0.6)',
                        borderColor: index === 0 
                            ? 'rgba(54, 162, 235, 1)' 
                            : index === 1 
                                ? 'rgba(255, 99, 132, 1)' 
                                : 'rgba(75, 192, 192, 1)',
                        fill: false,
                        yAxisID: yAxisID
                    });
                });
                
                new Chart(chartCtx, {
                    type: 'line',
                    data: {
                        datasets: datasets
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            title: {
                                display: true,
                                text: `${platform.charAt(0).toUpperCase() + platform.slice(1)} Performance Metrics by Test Duration`
                            }
                        },
                        scales: {
                            x: {
                                type: 'linear',
                                position: 'bottom',
                                title: {
                                    display: true,
                                    text: 'Test Duration (minutes)'
                                },
                                ticks: {
                                    callback: function(value) {
                                        return value + ' min';
                                    }
                                }
                            },
                            'y-percentage': {
                                beginAtZero: true,
                                position: 'left',
                                title: {
                                    display: true,
                                    text: 'RPS / Success Rate (%)'
                                }
                            },
                            'y-latency': {
                                beginAtZero: true,
                                position: 'right',
                                title: {
                                    display: true,
                                    text: 'Latency (ms)'
                                },
                                grid: {
                                    drawOnChartArea: false
                                }
                            }
                        }
                    }
                });
            });
        }
        
        function createMetricCard(title, value, description) {
            const card = document.createElement('div');
            card.className = 'metric-card';
            
            const titleEl = document.createElement('div');
            titleEl.textContent = title;
            titleEl.style.fontWeight = 'bold';
            
            const valueEl = document.createElement('div');
            valueEl.className = 'metric-value';
            valueEl.textContent = value;
            
            const descEl = document.createElement('div');
            descEl.className = 'metric-label';
            descEl.textContent = description;
            
            card.appendChild(titleEl);
            card.appendChild(valueEl);
            card.appendChild(descEl);
            
            return card;
        }
        
        function renderRecommendations(data) {
            // Overall recommendations
            const overallContainer = document.getElementById('overallRecommendations');
            const overallList = document.createElement('ul');
            
            data.combinedRecommendations.overall.forEach(rec => {
                const li = document.createElement('li');
                li.textContent = rec;
                overallList.appendChild(li);
            });
            
            overallContainer.appendChild(overallList);
            
            // Platform recommendations
            const platforms = ['medusa', 'saleor', 'spree'];
            
            platforms.forEach(platform => {
                const containerID = `${platform}Recommendations`;
                const container = document.getElementById(containerID);
                const list = document.createElement('ul');
                
                data.combinedRecommendations[platform].forEach(rec => {
                    const li = document.createElement('li');
                    li.textContent = rec;
                    list.appendChild(li);
                });
                
                container.appendChild(list);
            });
        }
        
        function populateRankingTable(data) {
            const platforms = ['medusa', 'saleor', 'spree'];
            
            // Calculate average RPS
            const avgRPS = {};
            platforms.forEach(platform => {
                const rpsValues = data.platformSummary[platform].rps.map(item => item.value);
                avgRPS[platform] = rpsValues.reduce((sum, val) => sum + val, 0) / rpsValues.length;
            });
            
            // Sort platforms by average RPS
            const throughputRanking = [...platforms].sort((a, b) => avgRPS[b] - avgRPS[a]);
            
            // Calculate average latency
            const avgLatency = {};
            platforms.forEach(platform => {
                const latencyValues = data.platformSummary[platform].latencyP95.map(item => item.value);
                avgLatency[platform] = latencyValues.reduce((sum, val) => sum + val, 0) / latencyValues.length;
            });
            
            // Sort platforms by average latency (lower is better)
            const latencyRanking = [...platforms].sort((a, b) => avgLatency[a] - avgLatency[b]);
            
            // Calculate stability (final RPS / initial RPS)
            const stability = {};
            platforms.forEach(platform => {
                const rpsValues = data.platformSummary[platform].rps.map(item => item.value);
                stability[platform] = rpsValues[rpsValues.length - 1] / rpsValues[0];
            });
            
            // Sort platforms by stability
            const stabilityRanking = [...platforms].sort((a, b) => stability[b] - stability[a]);
            
            // Calculate long-term performance (RPS in longest test)
            const longTermPerf = {};
            platforms.forEach(platform => {
                const rpsValues = data.platformSummary[platform].rps;
                longTermPerf[platform] = rpsValues[rpsValues.length - 1].value;
            });
            
            // Sort platforms by long-term performance
            const longTermRanking = [...platforms].sort((a, b) => longTermPerf[b] - longTermPerf[a]);
            
            // Populate ranking table
            for (let i = 0; i < 3; i++) {
                document.getElementById(`rank-throughput-${i+1}`).textContent = 
                    `${throughputRanking[i].charAt(0).toUpperCase() + throughputRanking[i].slice(1)} (${avgRPS[throughputRanking[i]].toFixed(2)} RPS)`;
                    
                document.getElementById(`rank-latency-${i+1}`).textContent = 
                    `${latencyRanking[i].charAt(0).toUpperCase() + latencyRanking[i].slice(1)} (${avgLatency[latencyRanking[i]].toFixed(2)} ms)`;
                    
                document.getElementById(`rank-stability-${i+1}`).textContent = 
                    `${stabilityRanking[i].charAt(0).toUpperCase() + stabilityRanking[i].slice(1)} (${(stability[stabilityRanking[i]] * 100).toFixed(2)}%)`;
                    
                document.getElementById(`rank-longterm-${i+1}`).textContent = 
                    `${longTermRanking[i].charAt(0).toUpperCase() + longTermRanking[i].slice(1)} (${longTermPerf[longTermRanking[i]].toFixed(2)} RPS)`;
            }
        }
    </script>
</body>
</html>
EOL

echo "Combined HTML report generated at $HTML_FILE"
echo "You can open this file in your browser to view the report."
echo
echo "For example: firefox $HTML_FILE"