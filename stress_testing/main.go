package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
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
	}
}

// Metrics tracks test execution metrics
type Metrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	mutex              sync.RWMutex
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
	Metrics  *Metrics
	StopChan chan struct{}
	client   *http.Client
}

// NewPlatform creates a new platform instance with optimized HTTP client
func NewPlatform(config PlatformConfig) *Platform {
	// Create a custom dialer with shorter timeouts
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Configure transport for high-concurrency testing
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          3000,
		MaxIdleConnsPerHost:   1000,
		MaxConnsPerHost:       1000,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return &Platform{
		Config:   config,
		Metrics:  &Metrics{},
		StopChan: make(chan struct{}),
		client:   client,
	}
}

// ExecuteRequest performs a single request to the platform
func (p *Platform) ExecuteRequest(wg *sync.WaitGroup) {
	defer wg.Done()
	
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
	resp, err := p.client.Do(req)
	
	// Handle response
	if err != nil {
		p.Metrics.AddResult(false)
		return
	}

	// Read and discard body to properly reuse connections
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Check if request was successful
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	p.Metrics.AddResult(success)
}

// StressTest runs a high-RPS stress test against the platform
func StressTest(p *Platform, rps int, duration time.Duration) {
	fmt.Printf("Starting stress test for %s at %d RPS for %s\n", 
		p.Config.Name, rps, duration.String())

	// Calculate total requests
	totalRequests := int(duration.Seconds()) * rps
	fmt.Printf("Will send %d total requests to %s\n", totalRequests, p.Config.Name)

	// For precise timing
	ticker := time.NewTicker(time.Second / time.Duration(rps))
	defer ticker.Stop()

	// Set up reporting
	reportTicker := time.NewTicker(1 * time.Second)
	defer reportTicker.Stop()

	// Set deadline
	deadline := time.Now().Add(duration)
	
	// WaitGroup for tracking in-flight requests
	var wg sync.WaitGroup
	
	// Track progress
	var requestsSent int64
	
	// Report current status
	go func() {
		lastReported := int64(0)
		for {
			select {
			case <-reportTicker.C:
				current := atomic.LoadInt64(&requestsSent)
				currentReqs := atomic.LoadInt64(&p.Metrics.TotalRequests)
				rate := current - lastReported
				lastReported = current
				percent := float64(current) / float64(totalRequests) * 100
				fmt.Printf("%s: %d/%d requests (%.1f%%) - Sent: %d RPS, Completed: %d\n", 
					p.Config.Name, current, totalRequests, percent, rate, currentReqs)
			case <-p.StopChan:
				return
			}
		}
	}()

	// Send requests at the specified rate
	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
			wg.Add(1)
			atomic.AddInt64(&requestsSent, 1)
			go p.ExecuteRequest(&wg)
		case <-p.StopChan:
			fmt.Printf("%s: Test interrupted\n", p.Config.Name)
			wg.Wait()
			return
		}
	}

	// Wait for any remaining requests to complete
	fmt.Printf("%s: All requests sent, waiting for completion...\n", p.Config.Name)
	wg.Wait()
	fmt.Printf("%s: Test completed. Sent %d requests, processed %d responses\n", 
		p.Config.Name, requestsSent, p.Metrics.TotalRequests)
}

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "stress_test_config.json", "Path to the configuration file")
	flag.Parse()

	// Set GOMAXPROCS to use all available CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())
	
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

	// Create a goroutine to handle the interrupt signal
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		close(saleor.StopChan)
		close(medusa.StopChan)
	}()

	// Set test parameters
	testDuration := time.Duration(config.Test.DurationSeconds) * time.Second
	rps := config.Test.RPS

	// Run tests in parallel
	var wg sync.WaitGroup
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		StressTest(saleor, rps, testDuration)
	}()
	
	go func() {
		defer wg.Done()
		StressTest(medusa, rps, testDuration)
	}()

	// Wait for both tests to complete
	wg.Wait()

	// Print comparison results
	fmt.Println("\n----- ERROR RATE COMPARISON RESULTS -----")
	fmt.Printf("Test Duration: %d seconds at target %d RPS\n\n", 
		config.Test.DurationSeconds, config.Test.RPS)

	saleorErrorRate := saleor.Metrics.GetErrorRate()
	medusaErrorRate := medusa.Metrics.GetErrorRate()

	fmt.Printf("Saleor:\n")
	fmt.Printf("  Total Requests Processed: %d\n", saleor.Metrics.TotalRequests)
	fmt.Printf("  Success Rate: %.2f%%\n", saleor.Metrics.GetSuccessRate())
	fmt.Printf("  Error Rate: %.2f%%\n\n", saleorErrorRate)

	fmt.Printf("Medusa:\n")
	fmt.Printf("  Total Requests Processed: %d\n", medusa.Metrics.TotalRequests)
	fmt.Printf("  Success Rate: %.2f%%\n", medusa.Metrics.GetSuccessRate())
	fmt.Printf("  Error Rate: %.2f%%\n\n", medusaErrorRate)

	// Determine which platform performed better
	fmt.Println("Comparison:")
	errorRateDiff := math.Abs(saleorErrorRate - medusaErrorRate)
	
	if saleorErrorRate < medusaErrorRate {
		fmt.Printf("Saleor has a lower error rate by %.2f percentage points\n", errorRateDiff)
	} else if medusaErrorRate < saleorErrorRate {
		fmt.Printf("Medusa has a lower error rate by %.2f percentage points\n", errorRateDiff)
	} else {
		fmt.Println("Both platforms have the same error rate")
	}

	// Save results to file
	results := map[string]interface{}{
		"testDuration": config.Test.DurationSeconds,
		"targetRPS":    config.Test.RPS,
		"saleor": map[string]interface{}{
			"totalRequests": saleor.Metrics.TotalRequests,
			"successRate":   saleor.Metrics.GetSuccessRate(),
			"errorRate":     saleorErrorRate,
		},
		"medusa": map[string]interface{}{
			"totalRequests": medusa.Metrics.TotalRequests,
			"successRate":   medusa.Metrics.GetSuccessRate(),
			"errorRate":     medusaErrorRate,
		},
		"comparisonResult": map[string]interface{}{
			"errorRateDifference": errorRateDiff,
			"betterPlatform": func() string {
				if saleorErrorRate < medusaErrorRate {
					return "Saleor"
				} else if medusaErrorRate < saleorErrorRate {
					return "Medusa"
				}
				return "Tie"
			}(),
		},
	}

	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	err = os.WriteFile("stress_test_results.json", resultsJSON, 0644)
	if err != nil {
		fmt.Printf("Error writing results file: %v\n", err)
	} else {
		fmt.Println("Results saved to stress_test_results.json")
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
			products(first: 5, channel: "default-channel") {
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
	config.Test.RPS = 1000          // Target 1000 RPS

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