#!/bin/bash

# Check if a results directory was provided
if [ $# -eq 0 ]; then
  echo "Usage: $0 <results_directory>"
  echo "Example: $0 benchmark_results_20250306_123456"
  exit 1
fi

RESULTS_DIR=$1

# Check if the directory exists
if [ ! -d "$RESULTS_DIR" ]; then
  echo "Error: Directory $RESULTS_DIR does not exist"
  exit 1
fi

# Check if the comparison JSON exists
if [ ! -f "$RESULTS_DIR/comparison.json" ]; then
  echo "Error: Comparison results file not found at $RESULTS_DIR/comparison.json"
  exit 1
fi

# Create an HTML file to display the results
HTML_FILE="$RESULTS_DIR/report.html"

cat > $HTML_FILE << 'EOL'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>E-commerce Platform Benchmark Results</title>
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
        .recommendations {
            background-color: #f0f7ff;
            padding: 20px;
            border-radius: 8px;
            margin-top: 30px;
        }
        .platform-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .platform-metrics {
            display: flex;
            flex-wrap: wrap;
            gap: 20px;
            margin-top: 20px;
        }
        .metric-card {
            flex: 1;
            min-width: 200px;
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 8px;
            box-shadow: 0 1px 5px rgba(0,0,0,0.05);
        }
        .metric-value {
            font-size: 24px;
            font-weight: bold;
            margin: 10px 0;
        }
        .metric-label {
            color: #666;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <h1>E-commerce Platform Benchmark Results</h1>
    
    <div class="card">
        <h2>Throughput Comparison</h2>
        <div class="chart-container">
            <canvas id="rpsChart"></canvas>
        </div>
    </div>

    <div class="card">
        <h2>Latency Comparison</h2>
        <div class="chart-container">
            <canvas id="latencyChart"></canvas>
        </div>
    </div>

    <div class="card">
        <h2>Success Rate Comparison</h2>
        <div class="chart-container">
            <canvas id="successRateChart"></canvas>
        </div>
    </div>

    <h2>Platform Summaries</h2>
    <div id="platformSummaries"></div>

    <div class="card">
        <h2>Detailed Metrics</h2>
        <table id="metricsTable">
            <thead>
                <tr>
                    <th>Metric</th>
                    <th>Medusa</th>
                    <th>Saleor</th>
                    <th>Spree</th>
                </tr>
            </thead>
            <tbody id="metricsTableBody">
                <!-- Data will be filled dynamically -->
            </tbody>
        </table>
    </div>

    <div class="recommendations">
        <h2>Recommendations</h2>
        <div id="recommendationsContent">
            <!-- Recommendations will be filled dynamically -->
        </div>
    </div>

    <script>
        // Load the comparison data
        fetch('comparison.json')
            .then(response => response.json())
            .then(data => {
                renderCharts(data);
                renderPlatformSummaries(data);
                renderTable(data);
                renderRecommendations(data);
            })
            .catch(error => {
                console.error('Error loading comparison data:', error);
                document.body.innerHTML += `<div style="color: red; padding: 20px;">Error loading data: ${error.message}</div>`;
            });

        function renderCharts(data) {
            // RPS Chart
            const rpsCtx = document.getElementById('rpsChart').getContext('2d');
            const rpsLabels = Object.keys(data.rpsComparison);
            const rpsValues = Object.values(data.rpsComparison).map(v => parseFloat(v));
            
            new Chart(rpsCtx, {
                type: 'bar',
                data: {
                    labels: rpsLabels,
                    datasets: [{
                        label: 'Requests Per Second',
                        data: rpsValues,
                        backgroundColor: [
                            'rgba(54, 162, 235, 0.6)',
                            'rgba(255, 99, 132, 0.6)',
                            'rgba(75, 192, 192, 0.6)'
                        ],
                        borderColor: [
                            'rgba(54, 162, 235, 1)',
                            'rgba(255, 99, 132, 1)',
                            'rgba(75, 192, 192, 1)'
                        ],
                        borderWidth: 1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Throughput (Requests Per Second)'
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

            // Latency Chart
            const latencyCtx = document.getElementById('latencyChart').getContext('2d');
            const platforms = Object.keys(data.latencyComparison);
            const p50Values = platforms.map(p => data.latencyComparison[p].p50 || 0);
            const p95Values = platforms.map(p => data.latencyComparison[p].p95 || 0);
            const p99Values = platforms.map(p => data.latencyComparison[p].p99 || 0);
            
            new Chart(latencyCtx, {
                type: 'bar',
                data: {
                    labels: platforms,
                    datasets: [
                        {
                            label: 'P50 Latency (ms)',
                            data: p50Values,
                            backgroundColor: 'rgba(54, 162, 235, 0.6)',
                            borderColor: 'rgba(54, 162, 235, 1)',
                            borderWidth: 1
                        },
                        {
                            label: 'P95 Latency (ms)',
                            data: p95Values,
                            backgroundColor: 'rgba(255, 159, 64, 0.6)',
                            borderColor: 'rgba(255, 159, 64, 1)',
                            borderWidth: 1
                        },
                        {
                            label: 'P99 Latency (ms)',
                            data: p99Values,
                            backgroundColor: 'rgba(255, 99, 132, 0.6)',
                            borderColor: 'rgba(255, 99, 132, 1)',
                            borderWidth: 1
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Response Latency (lower is better)'
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'Milliseconds'
                            }
                        }
                    }
                }
            });

            // Success Rate Chart
            const successRateCtx = document.getElementById('successRateChart').getContext('2d');
            const successRateLabels = Object.keys(data.errorComparison);
            const successRateValues = Object.keys(data.errorComparison).map(key => 100 - data.errorComparison[key]);
            
            new Chart(successRateCtx, {
                type: 'bar',
                data: {
                    labels: successRateLabels,
                    datasets: [{
                        label: 'Success Rate (%)',
                        data: successRateValues,
                        backgroundColor: [
                            'rgba(75, 192, 192, 0.6)',
                            'rgba(75, 192, 192, 0.6)',
                            'rgba(75, 192, 192, 0.6)'
                        ],
                        borderColor: [
                            'rgba(75, 192, 192, 1)',
                            'rgba(75, 192, 192, 1)',
                            'rgba(75, 192, 192, 1)'
                        ],
                        borderWidth: 1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Request Success Rate'
                        },
                        legend: {
                            display: false
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
        }

        function renderPlatformSummaries(data) {
            const container = document.getElementById('platformSummaries');
            
            Object.entries(data.platformData).forEach(([platform, metrics]) => {
                const card = document.createElement('div');
                card.className = 'card';
                
                const header = document.createElement('div');
                header.className = 'platform-header';
                
                const title = document.createElement('h3');
                title.textContent = platform.charAt(0).toUpperCase() + platform.slice(1);
                
                header.appendChild(title);
                card.appendChild(header);
                
                const metricsDiv = document.createElement('div');
                metricsDiv.className = 'platform-metrics';
                
                // Add RPS metric
                const rpsMetric = createMetricCard('Throughput', `${data.rpsComparison[platform]} RPS`, 'Requests per second');
                metricsDiv.appendChild(rpsMetric);
                
                // Add success rate metric
                const successRate = 100 - data.errorComparison[platform];
                const successMetric = createMetricCard('Success Rate', `${successRate.toFixed(2)}%`, 'Percentage of successful requests');
                metricsDiv.appendChild(successMetric);
                
                // Add P95 latency metric if available
                if (data.latencyComparison[platform] && data.latencyComparison[platform].p95) {
                    const latencyMetric = createMetricCard('P95 Latency', `${data.latencyComparison[platform].p95} ms`, '95th percentile response time');
                    metricsDiv.appendChild(latencyMetric);
                }
                
                // Add total requests metric
                if (metrics.totalRequests) {
                    const requestsMetric = createMetricCard('Total Requests', metrics.totalRequests, 'Number of requests made');
                    metricsDiv.appendChild(requestsMetric);
                }
                
                card.appendChild(metricsDiv);
                container.appendChild(card);
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

        function renderTable(data) {
            const tableBody = document.getElementById('metricsTableBody');
            
            // Add rows from summary table
            data.summaryTable.forEach(row => {
                const tr = document.createElement('tr');
                
                const metricCell = document.createElement('td');
                metricCell.textContent = row.metric;
                tr.appendChild(metricCell);
                
                // Add a cell for each platform
                ['medusa', 'saleor', 'spree'].forEach(platform => {
                    const cell = document.createElement('td');
                    cell.textContent = row[platform] || 'N/A';
                    tr.appendChild(cell);
                });
                
                tableBody.appendChild(tr);
            });
        }

        function renderRecommendations(data) {
            const container = document.getElementById('recommendationsContent');
            
            // Overall recommendations
            const overallTitle = document.createElement('h3');
            overallTitle.textContent = 'Overall Recommendations';
            container.appendChild(overallTitle);
            
            const overallList = document.createElement('ul');
            data.recommendations.overall.forEach(rec => {
                const li = document.createElement('li');
                li.textContent = rec;
                overallList.appendChild(li);
            });
            container.appendChild(overallList);
            
            // Platform-specific recommendations
            Object.entries(data.recommendations).forEach(([platform, recs]) => {
                if (platform === 'overall') return;
                
                const platformTitle = document.createElement('h3');
                platformTitle.textContent = `${platform.charAt(0).toUpperCase() + platform.slice(1)} Recommendations`;
                container.appendChild(platformTitle);
                
                const platformList = document.createElement('ul');
                recs.forEach(rec => {
                    const li = document.createElement('li');
                    li.textContent = rec;
                    platformList.appendChild(li);
                });
                container.appendChild(platformList);
            });
        }
    </script>
</body>
</html>
EOL

echo "HTML report generated at $HTML_FILE"
echo "You can open this file in your browser to view the report."
echo
echo "For example: firefox $HTML_FILE"