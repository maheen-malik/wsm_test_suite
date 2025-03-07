package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Command line flags
var (
	medusaPath                 = flag.String("medusa", "", "Path to Medusa results JSON file")
	saleorPath                 = flag.String("saleor", "", "Path to Saleor results JSON file")
	spreePath                  = flag.String("spree", "", "Path to Spree results JSON file")
	outputPath = flag.String("output", "comparison.json", "Path to output comparison JSON file")
)

// PlatformResults represents the benchmark results for a single platform
type PlatformResults map[string]interface{}

// ComparisonResults represents the comparison results for all platforms
type ComparisonResults struct {
	Timestamp         string                     `json:"timestamp"`
	PlatformData      map[string]PlatformResults `json:"platformData"`
	SummaryTable      []map[string]interface{}   `json:"summaryTable"`
	RPSComparison     map[string]interface{}     `json:"rpsComparison"`
	LatencyComparison map[string]interface{}     `json:"latencyComparison"`
	ErrorComparison   map[string]interface{}     `json:"errorComparison"`
	Recommendations   map[string][]string        `json:"recommendations"`
}

// LoadResults loads benchmark results from a JSON file
func LoadResults(path string) (PlatformResults, error) {
	if path == "" {
		return nil, fmt.Errorf("no path provided")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", path, err)
	}

	var results PlatformResults
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("error parsing JSON from %s: %v", path, err)
	}

	return results, nil
}

// ExtractNumericValue extracts a numeric value from a string (e.g., "52.65" from "52.65 req/s")
func ExtractNumericValue(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		// Extract numeric part and convert to float
		numStr := v
		// Remove any units or % signs
		numStr = strings.Split(numStr, " ")[0]
		numStr = strings.TrimSuffix(numStr, "%")

		var result float64
		_, err := fmt.Sscanf(numStr, "%f", &result)
		if err != nil {
			return 0
		}
		return result
	default:
		return 0
	}
}

// ParseDuration parses a duration string (e.g., "125.3ms") into milliseconds
func ParseDuration(durationStr string) float64 {
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0
	}
	return float64(d) / float64(time.Millisecond)
}

// GetSuccessRate extracts the success rate percentage from results
func GetSuccessRate(results PlatformResults) float64 {
	if rateStr, ok := results["successRate"].(string); ok {
		return ExtractNumericValue(rateStr)
	}
	return 0
}

// GetActualRPS extracts the actual RPS value from results
func GetActualRPS(results PlatformResults) float64 {
	if rpsStr, ok := results["actualRPS"].(string); ok {
		return ExtractNumericValue(rpsStr)
	}
	return 0
}

// GetLatencyP95 extracts the p95 latency value from results in milliseconds
func GetLatencyP95(results PlatformResults) float64 {
	latency, ok := results["latency"].(map[string]interface{})
	if !ok {
		return 0
	}

	if p95Str, ok := latency["p95"].(string); ok {
		return ParseDuration(p95Str)
	}

	return 0
}

// BuildSummaryTable creates a summary table for all platforms
func BuildSummaryTable(platforms map[string]PlatformResults) []map[string]interface{} {
	metrics := []string{
		"actualRPS",
		"successRate",
		"testDuration",
		"totalRequests",
		"successfulRequests",
		"failedRequests",
	}

	var result []map[string]interface{}

	// Add standard metrics
	for _, metric := range metrics {
		row := map[string]interface{}{
			"metric": metric,
		}

		for platform, data := range platforms {
			row[platform] = data[metric]
		}

		result = append(result, row)
	}

	// Add latency metrics
	latencyMetrics := []string{"p50", "p90", "p95", "p99"}
	for _, lm := range latencyMetrics {
		row := map[string]interface{}{
			"metric": "latency_" + lm,
		}

		for platform, data := range platforms {
			if latency, ok := data["latency"].(map[string]interface{}); ok {
				row[platform] = latency[lm]
			} else {
				row[platform] = "N/A"
			}
		}

		result = append(result, row)
	}

	return result
}

// GenerateRecommendations creates recommendations based on the benchmark results
func GenerateRecommendations(platforms map[string]PlatformResults) map[string][]string {
	recommendations := make(map[string][]string)

	// Sort platforms by RPS for easier comparison
	type platformRPS struct {
		Name string
		RPS  float64
	}

	var platformsSorted []platformRPS
	for name, results := range platforms {
		platformsSorted = append(platformsSorted, platformRPS{
			Name: name,
			RPS:  GetActualRPS(results),
		})
	}

	sort.Slice(platformsSorted, func(i, j int) bool {
		return platformsSorted[i].RPS > platformsSorted[j].RPS
	})

	// Generate overall recommendations
	recommendations["overall"] = []string{
		fmt.Sprintf("%s achieved the highest throughput at %.2f RPS",
			platformsSorted[0].Name, platformsSorted[0].RPS),
	}

	// Generate platform-specific recommendations
	for name, results := range platforms {
		rps := GetActualRPS(results)
		successRate := GetSuccessRate(results)
		p95Latency := GetLatencyP95(results)

		platformRecs := []string{}

		// Add RPS recommendations
		if rps < 50 {
			platformRecs = append(platformRecs,
				"Consider optimizing for higher throughput, current RPS is relatively low")
		}

		// Add success rate recommendations
		if successRate < 95 {
			platformRecs = append(platformRecs,
				fmt.Sprintf("Investigate failed requests, success rate is %.1f%%", successRate))
		}

		// Add latency recommendations
		if p95Latency > 2000 {
			platformRecs = append(platformRecs,
				fmt.Sprintf("P95 latency is high (%.0f ms), consider performance tuning", p95Latency))
		}

		// If everything looks good
		if len(platformRecs) == 0 {
			platformRecs = append(platformRecs, "Performance metrics look good")
		}

		recommendations[name] = platformRecs
	}

	return recommendations
}

func main() {
	flag.Parse()

	// Load results for each platform
	medusaResults, medusaErr := LoadResults(*medusaPath)
	saleorResults, saleorErr := LoadResults(*saleorPath)
	spreeResults, spreeErr := LoadResults(*spreePath)

	// Check if at least one platform's results loaded successfully
	if medusaErr != nil && saleorErr != nil && spreeErr != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load any platform results\n")
		os.Exit(1)
	}

	// Collect all platform results
	platforms := make(map[string]PlatformResults)

	if medusaErr == nil {
		platforms["medusa"] = medusaResults
	} else {
		fmt.Fprintf(os.Stderr, "Warning: Could not load Medusa results: %v\n", medusaErr)
	}

	if saleorErr == nil {
		platforms["saleor"] = saleorResults
	} else {
		fmt.Fprintf(os.Stderr, "Warning: Could not load Saleor results: %v\n", saleorErr)
	}

	if spreeErr == nil {
		platforms["spree"] = spreeResults
	} else {
		fmt.Fprintf(os.Stderr, "Warning: Could not load Spree results: %v\n", spreeErr)
	}

	// Create comparison
	comparison := ComparisonResults{
		Timestamp:    time.Now().Format(time.RFC3339),
		PlatformData: platforms,
		SummaryTable: BuildSummaryTable(platforms),
	}

	// Extract RPS comparison
	rpsComparison := make(map[string]interface{})
	for platform, results := range platforms {
		rpsComparison[platform] = GetActualRPS(results)
	}
	comparison.RPSComparison = rpsComparison

	// Extract latency comparison
	latencyComparison := make(map[string]interface{})
	for platform, results := range platforms {
		latency := make(map[string]interface{})
		if latencyData, ok := results["latency"].(map[string]interface{}); ok {
			for metric, value := range latencyData {
				latency[metric] = ParseDuration(value.(string))
			}
		}
		latencyComparison[platform] = latency
	}
	comparison.LatencyComparison = latencyComparison

	// Extract error comparison
	errorComparison := make(map[string]interface{})
	for platform, results := range platforms {
		errorRate := 100 - GetSuccessRate(results)
		errorComparison[platform] = errorRate
	}
	comparison.ErrorComparison = errorComparison

	// Generate recommendations
	comparison.Recommendations = GenerateRecommendations(platforms)

	// Write comparison to file
	comparisonData, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal comparison results: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(*outputPath, comparisonData, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write comparison results: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Comparison results written to %s\n", *outputPath)

	// Print summary to console
	fmt.Println("\nPlatform Performance Summary:")
	fmt.Println("----------------------------")

	for platform, rps := range rpsComparison {
		fmt.Printf("%s: %.2f RPS, ", platform, rps)
		fmt.Printf("%.2f%% Success Rate, ", 100-errorComparison[platform].(float64))

		latency := latencyComparison[platform].(map[string]interface{})
		if p95, ok := latency["p95"]; ok {
			fmt.Printf("P95 Latency: %.0f ms\n", p95)
		} else {
			fmt.Println("P95 Latency: N/A")
		}
	}
}
