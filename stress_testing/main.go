package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// PlatformConfig holds configuration for a specific platform
type PlatformConfig struct {
	Name      string
	URL       string
	Headers   map[string]string
	Query     string
	IsGraphQL bool
}

// Config holds the application configuration
type Config struct {
	Saleor PlatformConfig
	Medusa PlatformConfig
	Test   struct {
		DurationSeconds int
		RPS             int
		Workers         int
	}
}

// Metrics tracks test execution metrics
type Metrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	mutex              sync.RWMutex
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// AddResult adds a result to the metrics
func (m *Metrics) AddResult(success bool) {
	atomic.AddInt64(&m.TotalRequests, 1)
	if success {
		atomic.AddInt64(&m.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&m.FailedRequests, 1)
	}
}

// GetSuccessRate returns the success rate as a percentage
func (m *Metrics) GetSuccessRate() float64 {
	if m.TotalRequests == 0 {
		return 100.0
	}
	return float64(m.SuccessfulRequests) / float64(m.TotalRequests) * 100.0
}

// GetErrorRate returns the error rate as a percentage
func (m *Metrics) GetErrorRate() float64 {
	if m.TotalRequests == 0 {
		return 0.0
	}
	return float64(m.FailedRequests) / float64(m.TotalRequests) * 100.0
}

// Platform represents an e-commerce platform to test
type Platform struct {
	Config   PlatformConfig
	Client   *http.Client
	Metrics  *Metrics
	StopChan chan struct{}
}

// NewPlatform creates a new platform instance
func NewPlatform(config PlatformConfig) *Platform {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return &Platform{
		Config:   config,
		Client:   client,
		Metrics:  NewMetrics(),
		StopChan: make(chan struct{}),
	}
}

// Execute a request to the platform
func (p *Platform) ExecuteRequest() {
	var req *http.Request
	var err error

	if p.Config.IsGraphQL {
		// Prepare GraphQL request
		graphqlReq := map[string]interface{}{
			"query": p.Config.Query,
		}

		reqBody, err := json.Marshal(graphqlReq)
		if err != nil {
			p.Metrics.AddResult(false)
			return
		}

		req, err = http.NewRequest("POST", p.Config.URL, bytes.NewBuffer(reqBody))
	} else {
		// REST request
		req, err = http.NewRequest("GET", p.Config.URL, nil)
	}

	if err != nil {
		p.Metrics.AddResult(false)
		return
	}

	// Add headers
	for key, value := range p.Config.Headers {
		req.Header.Set(key, value)
	}

	// Execute request
	resp, err := p.Client.Do(req)
	if err != nil {
		p.Metrics.AddResult(false)
		return
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Check if request was successful
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	p.Metrics.AddResult(success)
}

// RunTest executes the load test for the platform
func (p *Platform) RunTest(rps int, duration time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Starting test for %s at %d RPS for %v\n", p.Config.Name, rps, duration)

	ticker := time.NewTicker(time.Second / time.Duration(rps))
	defer ticker.Stop()

	timeout := time.After(duration)

	for {
		select {
		case <-ticker.C:
			go p.ExecuteRequest()
		case <-timeout:
			fmt.Printf("Test completed for %s\n", p.Config.Name)
			return
		case <-p.StopChan:
			return
		}
	}
}

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "error_rate_config.json", "Path to the configuration file")
	flag.Parse()

	// Load or create configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create platforms
	saleor := NewPlatform(config.Saleor)
	medusa := NewPlatform(config.Medusa)

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the tests
	var wg sync.WaitGroup
	wg.Add(2)

	testDuration := time.Duration(config.Test.DurationSeconds) * time.Second
	go saleor.RunTest(config.Test.RPS, testDuration, &wg)
	go medusa.RunTest(config.Test.RPS, testDuration, &wg)

	// Wait for interrupt or completion
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		close(saleor.StopChan)
		close(medusa.StopChan)
	}()

	// Wait for tests to complete
	wg.Wait()

	// Print comparison results
	fmt.Println("\n----- ERROR RATE COMPARISON RESULTS -----")
	fmt.Printf("Test Duration: %d seconds at %d RPS\n\n", config.Test.DurationSeconds, config.Test.RPS)

	saleorSuccessRate := saleor.Metrics.GetSuccessRate()
	saleorErrorRate := saleor.Metrics.GetErrorRate()
	medusaSuccessRate := medusa.Metrics.GetSuccessRate()
	medusaErrorRate := medusa.Metrics.GetErrorRate()

	fmt.Printf("Saleor:\n")
	fmt.Printf("  Total Requests: %d\n", saleor.Metrics.TotalRequests)
	fmt.Printf("  Success Rate: %.2f%%\n", saleorSuccessRate)
	fmt.Printf("  Error Rate: %.2f%%\n\n", saleorErrorRate)

	fmt.Printf("Medusa:\n")
	fmt.Printf("  Total Requests: %d\n", medusa.Metrics.TotalRequests)
	fmt.Printf("  Success Rate: %.2f%%\n", medusaSuccessRate)
	fmt.Printf("  Error Rate: %.2f%%\n\n", medusaErrorRate)

	// Determine which platform performed better
	fmt.Println("Comparison:")
	if saleorErrorRate < medusaErrorRate {
		fmt.Printf("Saleor has a lower error rate by %.2f percentage points\n", medusaErrorRate-saleorErrorRate)
	} else if medusaErrorRate < saleorErrorRate {
		fmt.Printf("Medusa has a lower error rate by %.2f percentage points\n", saleorErrorRate-medusaErrorRate)
	} else {
		fmt.Println("Both platforms have the same error rate")
	}

	// Save results to file
	results := map[string]interface{}{
		"testDuration": config.Test.DurationSeconds,
		"rps":          config.Test.RPS,
		"saleor": map[string]interface{}{
			"totalRequests": saleor.Metrics.TotalRequests,
			"successRate":   saleorSuccessRate,
			"errorRate":     saleorErrorRate,
		},
		"medusa": map[string]interface{}{
			"totalRequests": medusa.Metrics.TotalRequests,
			"successRate":   medusaSuccessRate,
			"errorRate":     medusaErrorRate,
		},
	}

	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	err = os.WriteFile("error_rate_results.json", resultsJSON, 0644)
	if err != nil {
		fmt.Printf("Error writing results file: %v\n", err)
	} else {
		fmt.Println("Results saved to error_rate_results.json")
	}
}

// loadConfig loads the configuration from a file or creates a default one
func loadConfig(path string) (*Config, error) {
	configFile, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return createDefaultConfig(path)
		}
		return nil, err
	}
	defer configFile.Close()

	var config Config
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// createDefaultConfig creates a default configuration file
func createDefaultConfig(path string) (*Config, error) {
	config := &Config{}

	// Saleor configuration
	config.Saleor = PlatformConfig{
		Name:      "Saleor",
		URL:       "https://wsm-saleor.alphasquadit.com/graphql/",
		IsGraphQL: true,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Query: `{
			products(first: 10, channel: "default-channel") {
				edges {
					node {
						id
						name
					}
				}
			}
		}`,
	}

	// Medusa configuration
	config.Medusa = PlatformConfig{
		Name:      "Medusa",
		URL:       "http://wsm-medusa.alphasquadit.com/store/products",
		IsGraphQL: false,
		Headers: map[string]string{
			"Accept":                "application/json",
			"Content-Type":          "application/json",
			"x-publishable-api-key": "pk_cf8ea2bcf8f97ee114ed8797b464ffb068777ff1751ac7b0612f58b06dca21fa",
		},
	}

	// Test configuration
	config.Test.DurationSeconds = 60 // 1 minute
	config.Test.RPS = 1
	config.Test.Workers = 10

	// Write configuration to file
	configFile, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return nil, err
	}

	fmt.Printf("Default configuration created at %s\n", path)
	return config, nil
}