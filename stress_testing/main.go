package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
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
		TotalRequests   int // Total requests to send
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
	// Configure transport for high-performance, high-concurrency testing
	transport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		MaxConnsPerHost:     1000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second, // Shorter timeout to fail faster
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

// RunFixedRequestCountTest sends exactly the specified number of requests
func (p *Platform) RunFixedRequestCountTest(totalRequests int, targetRPS int, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Starting test for %s with %d total requests at target rate of %d RPS\n", 
		p.Config.Name, totalRequests, targetRPS)

	// Create a worker pool
	numWorkers := 200
	
	// Create a channel to distribute work
	tasks := make(chan struct{}, numWorkers*2)
	
	// Launch workers
	workerWg := sync.WaitGroup{}
	for i := 0; i < numWorkers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for range tasks {
				p.ExecuteRequest()
			}
		}()
	}
	
	// Used for rate limiting
	interval := time.Second / time.Duration(targetRPS)
	nextRequestTime := time.Now()
	
	// Initialize progress tracking
	lastReportTime := time.Now()
	lastReportCount := int64(0)
	
	// Send exactly the requested number of requests
	for i := 0; i < totalRequests; i++ {
		// Check for stop signal
		select {
		case <-p.StopChan:
			fmt.Printf("Test for %s stopped early after %d requests\n", p.Config.Name, i)
			close(tasks)
			workerWg.Wait()
			return
		default:
			// Continue with test
		}
		
		// Simple rate limiting
		now := time.Now()
		if now.Before(nextRequestTime) {
			sleepTime := nextRequestTime.Sub(now)
			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
		}
		nextRequestTime = time.Now().Add(interval)
		
		// Submit task
		tasks <- struct{}{}
		
		// Report progress every second
		if time.Since(lastReportTime) >= time.Second {
			currentCount := atomic.LoadInt64(&p.Metrics.TotalRequests)
			rps := currentCount - lastReportCount
			lastReportCount = currentCount
			
			percentComplete := float64(currentCount) / float64(totalRequests) * 100
			fmt.Printf("%s: %d/%d requests (%.1f%%) - Current rate: %d RPS\n", 
				p.Config.Name, currentCount, totalRequests, percentComplete, rps)
				
			lastReportTime = time.Now()
		}
	}
	
	// Wait for all tasks to complete
	close(tasks)
	workerWg.Wait()
	
	fmt.Printf("Test completed for %s - Total requests: %d\n", 
		p.Config.Name, atomic.LoadInt64(&p.Metrics.TotalRequests))
}

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "error_rate_config.json", "Path to the configuration file")
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

	// Start the tests
	var wg sync.WaitGroup
	wg.Add(2)

	// Calculate the exact number of requests to send
	totalRequests := config.Test.RPS * config.Test.DurationSeconds

	go saleor.RunFixedRequestCountTest(totalRequests, config.Test.RPS, &wg)
	go medusa.RunFixedRequestCountTest(totalRequests, config.Test.RPS, &wg)

	// Wait for tests to complete
	wg.Wait()

	// Print comparison results
	fmt.Println("\n----- ERROR RATE COMPARISON RESULTS -----")
	fmt.Printf("Test completed: %d requests at target %d RPS over %d seconds\n\n", 
		totalRequests, config.Test.RPS, config.Test.DurationSeconds)

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
		"targetRPS":    config.Test.RPS,
		"totalRequestsPerPlatform": totalRequests,
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
		"comparison": map[string]interface{}{
			"errorRateDifference": math.Abs(saleorErrorRate - medusaErrorRate),
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
	config.Test.RPS = 1000          // Target 1000 RPS
	config.Test.TotalRequests = 60000 // 60 seconds * 1000 RPS

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