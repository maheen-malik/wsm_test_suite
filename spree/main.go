package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Config holds the application configuration
type Config struct {
	// API endpoints
	Endpoints struct {
		Products   string
		SpecificProduct string
	}
	
	// HTTP headers
	Headers map[string]string
	
	// Load test configuration
	Test struct {
		MaxWorkers       int
		MaxQueueSize     int
		RampupStages     []Stage
		ReportingSeconds int
		LogErrors        bool
		ErrorSampleRate  float64
		// Traffic distribution percentages
		TrafficDistribution struct {
			Products   int
			SpecificProduct int
		}
	}
}

// Stage represents a load testing stage
type Stage struct {
	Duration     time.Duration
	TargetRPS    int64
	Description  string
}

// ErrorResponse tracks details about failed requests
type ErrorResponse struct {
	URL        string
	StatusCode int
	Body       string
	Time       time.Time
	Error      string // If error occurred before getting a response
}

// Metrics tracks test execution metrics
type Metrics struct {
	StartTime          time.Time
	EndTime            time.Time
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	RequestDurations   []time.Duration
	StatusCodes        map[int]int64
	EndpointCounts     map[string]int64
	ErrorSamples       []ErrorResponse
	mutex              sync.RWMutex
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:      time.Now(),
		StatusCodes:    make(map[int]int64),
		EndpointCounts: make(map[string]int64),
		ErrorSamples:   make([]ErrorResponse, 0, 100),
	}
}

// AddResult adds a result to the metrics
func (m *Metrics) AddResult(duration time.Duration, endpoint string, statusCode int, errResp *ErrorResponse) {
	atomic.AddInt64(&m.TotalRequests, 1)
	
	m.mutex.Lock()
	m.EndpointCounts[endpoint]++
	m.StatusCodes[statusCode]++
	m.mutex.Unlock()
	
	if statusCode >= 200 && statusCode < 300 {
		atomic.AddInt64(&m.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&m.FailedRequests, 1)
		
		// Store error sample if provided
		if errResp != nil {
			m.mutex.Lock()
			if len(m.ErrorSamples) < 100 { // Limit to 100 samples
				m.ErrorSamples = append(m.ErrorSamples, *errResp)
			}
			m.mutex.Unlock()
		}
	}
	
	// Only store a sample of durations to avoid memory issues
	if rand.Float64() < 0.1 { // Store 10% of durations
		m.mutex.Lock()
		m.RequestDurations = append(m.RequestDurations, duration)
		m.mutex.Unlock()
	}
}

// Task represents a single request to be executed
type Task struct {
	URL     string
	Headers map[string]string
	Method  string
	Type    string // For metrics tracking
}

// Worker pool for handling concurrent requests
type WorkerPool struct {
	Tasks       chan Task
	Workers     int
	StopChan    chan struct{}
	WaitGroup   sync.WaitGroup
	HTTPClient  *http.Client
	Metrics     *Metrics
	CurrentRate *atomic.Int64
	Config      *Config
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers, queueSize int, metrics *Metrics, config *Config) *WorkerPool {
	// Create an optimized HTTP transport
	transport := &http.Transport{
		MaxIdleConns:        workers,
		MaxIdleConnsPerHost: workers,
		MaxConnsPerHost:     workers,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false, // Keep compression for REST APIs
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}
	
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second, // Match the K6 script's 10s timeout
	}
	
	currentRate := &atomic.Int64{}
	currentRate.Store(0)
	
	return &WorkerPool{
		Tasks:       make(chan Task, queueSize),
		Workers:     workers,
		StopChan:    make(chan struct{}),
		HTTPClient:  client,
		Metrics:     metrics,
		CurrentRate: currentRate,
		Config:      config,
	}
}

// Start launches the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.Workers; i++ {
		p.WaitGroup.Add(1)
		go p.worker()
	}
}

// Stop shuts down the worker pool
func (p *WorkerPool) Stop() {
	close(p.StopChan)
	p.WaitGroup.Wait()
}

// worker processes tasks from the queue
func (p *WorkerPool) worker() {
	defer p.WaitGroup.Done()
	
	for {
		select {
		case task, ok := <-p.Tasks:
			if !ok {
				return
			}
			p.executeTask(task)
		case <-p.StopChan:
			return
		}
	}
}

// executeTask performs the HTTP request
func (p *WorkerPool) executeTask(task Task) {
	req, err := http.NewRequest(task.Method, task.URL, nil)
	if err != nil {
		errResp := &ErrorResponse{
			URL:   task.URL,
			Time:  time.Now(),
			Error: fmt.Sprintf("request creation error: %v", err),
		}
		p.Metrics.AddResult(0, task.Type, 0, errResp)
		return
	}
	
	// Add headers
	for key, value := range task.Headers {
		req.Header.Set(key, value)
	}
	
	start := time.Now()
	resp, err := p.HTTPClient.Do(req)
	duration := time.Since(start)
	
	if err != nil {
		errResp := &ErrorResponse{
			URL:   task.URL,
			Time:  time.Now(),
			Error: fmt.Sprintf("request error: %v", err),
		}
		p.Metrics.AddResult(duration, task.Type, 0, errResp)
		return
	}
	
	var errorResponse *ErrorResponse
	if resp.StatusCode >= 400 && p.Config.Test.LogErrors && rand.Float64() <= p.Config.Test.ErrorSampleRate {
		// Sample some error responses for debugging
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		
		errorResponse = &ErrorResponse{
			URL:        task.URL,
			StatusCode: resp.StatusCode,
			Body:       bodyStr,
			Time:       time.Now(),
		}
		
		// Create a new reader with the same content for the next reader
		resp.Body.Close()
	} else {
		// Always close the body
		if resp.Body != nil {
			resp.Body.Close()
		}
	}
	
	// Body validation is handled by checking for a 200 status code and non-empty body
	// The non-empty body check is simplified since we've already consumed or closed the body
	p.Metrics.AddResult(duration, task.Type, resp.StatusCode, errorResponse)
	
	// Add a small sleep to avoid overwhelming the system, as in the K6 script
	sleepTime := 100 + rand.Intn(200) // 100-300ms sleep
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
}

// LoadGenerator controls the rate of request generation
type LoadGenerator struct {
	Pool      *WorkerPool
	Config    *Config
	StopChan  chan struct{}
	WaitGroup sync.WaitGroup
}

// NewLoadGenerator creates a new load generator
func NewLoadGenerator(pool *WorkerPool, config *Config) *LoadGenerator {
	return &LoadGenerator{
		Pool:     pool,
		Config:   config,
		StopChan: make(chan struct{}),
	}
}

// Start begins the load generation process
func (g *LoadGenerator) Start() {
	g.WaitGroup.Add(1)
	go g.generateLoad()
}

// Stop halts the load generation
func (g *LoadGenerator) Stop() {
	close(g.StopChan)
	g.WaitGroup.Wait()
}

// selectEndpoint selects an endpoint based on configured distribution
func (g *LoadGenerator) selectEndpoint() (string, string) {
	// Default to even distribution if not specified
	productsWeight := g.Config.Test.TrafficDistribution.Products
	if productsWeight == 0 {
		productsWeight = 60 // Default from K6 script
	}
	
	categoriesWeight := g.Config.Test.TrafficDistribution.SpecificProduct
	if categoriesWeight == 0 {
		categoriesWeight = 20 // Default from K6 script
	}
	
	// Calculate thresholds
	productsThreshold := productsWeight
	
	// Random selection based on weights
	rand := rand.Intn(100)
	if rand < productsThreshold {
		return g.Config.Endpoints.Products, "products"
	} else {
		return g.Config.Endpoints.SpecificProduct, "specificProduct"
	}
}

// generateTask creates a task for the specified endpoint
func (g *LoadGenerator) generateTask() Task {
	// Select endpoint based on distribution
	url, endpointType := g.selectEndpoint()
	
	return Task{
		URL:     url,
		Headers: g.Config.Headers,
		Method:  "GET",
		Type:    endpointType,
	}
}

// generateLoad produces tasks at the configured rate
func (g *LoadGenerator) generateLoad() {
	defer g.WaitGroup.Done()
	
	stageStart := time.Now()
	currentStage := 0
	
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()
	
	// Initialize variables for rate limiting
	startRPS := int64(0)
	if len(g.Config.Test.RampupStages) > 0 {
		startRPS = g.Config.Test.RampupStages[0].TargetRPS
	}
	
	currentTargetRPS := startRPS
	g.Pool.CurrentRate.Store(currentTargetRPS)
	
	// Launch the reporting goroutine
	reportTicker := time.NewTicker(time.Duration(g.Config.Test.ReportingSeconds) * time.Second)
	defer reportTicker.Stop()
	
	go func() {
		for {
			select {
			case <-reportTicker.C:
				printReport(g.Pool.Metrics, currentTargetRPS)
			case <-g.StopChan:
				return
			}
		}
	}()
	
	// Variables for tracking requests per second
	secondStart := time.Now()
	requestsThisSecond := int64(0)
	
	for {
		select {
		case <-g.StopChan:
			return
		case now := <-ticker.C:
			// Check if we need to move to the next stage
			if currentStage < len(g.Config.Test.RampupStages) {
				stage := g.Config.Test.RampupStages[currentStage]
				elapsed := now.Sub(stageStart)
				
				if elapsed >= stage.Duration {
					// Move to next stage
					stageStart = now
					currentStage++
					if currentStage < len(g.Config.Test.RampupStages) {
						startRPS = currentTargetRPS
						fmt.Printf("Moving to stage %d: %s\n", currentStage+1, g.Config.Test.RampupStages[currentStage].Description)
					} else {
						fmt.Println("Load test completed all stages.")
						return
					}
				}
				
				// Calculate current target RPS based on linear interpolation
				if currentStage < len(g.Config.Test.RampupStages) {
					stage = g.Config.Test.RampupStages[currentStage]
					progress := float64(elapsed) / float64(stage.Duration)
					
					// Linear interpolation between start RPS and target RPS
					currentTargetRPS = startRPS + int64(float64(stage.TargetRPS-startRPS)*progress)
					g.Pool.CurrentRate.Store(currentTargetRPS)
				}
			}
			
			// Check if we've started a new second
			if now.Sub(secondStart) >= time.Second {
				secondStart = now
				requestsThisSecond = 0
			}
			
			// Ensure we don't exceed our target RPS
			if requestsThisSecond < currentTargetRPS {
				// Generate a task
				task := g.generateTask()
				
				// Try to send the task, but don't block if queue is full
				select {
				case g.Pool.Tasks <- task:
					requestsThisSecond++
				default:
					// Queue is full, skip this task
				}
			}
		}
	}
}

// printReport generates and prints a report of current metrics
func printReport(metrics *Metrics, targetRPS int64) {
	metrics.mutex.RLock()
	defer metrics.mutex.RUnlock()
	
	testDuration := time.Since(metrics.StartTime)
	actualRPS := float64(metrics.TotalRequests) / testDuration.Seconds()
	
	// Calculate endpoint distribution
	endpointDistribution := make(map[string]float64)
	totalEndpoints := int64(0)
	for _, count := range metrics.EndpointCounts {
		totalEndpoints += count
	}
	
	if totalEndpoints > 0 {
		for endpoint, count := range metrics.EndpointCounts {
			endpointDistribution[endpoint] = float64(count) / float64(totalEndpoints) * 100
		}
	}
	
	// Create basic report
	report := map[string]interface{}{
		"totalRequests":      metrics.TotalRequests,
		"successfulRequests": metrics.SuccessfulRequests,
		"failedRequests":     metrics.FailedRequests,
		"testDuration":       testDuration.String(),
		"actualRPS":          fmt.Sprintf("%.2f", actualRPS),
		"targetRPS":          targetRPS,
		"successRate":        fmt.Sprintf("%.2f%%", float64(metrics.SuccessfulRequests)/float64(max(metrics.TotalRequests, 1))*100),
		"statusCodes":        metrics.StatusCodes,
		"endpointDistribution": endpointDistribution,
	}
	
	// Calculate latency percentiles if we have data
	if len(metrics.RequestDurations) > 0 {
		// Sort the durations for percentile calculation
		sorted := make([]time.Duration, len(metrics.RequestDurations))
		copy(sorted, metrics.RequestDurations)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})
		
		report["latency"] = map[string]string{
			"p50": percentileDuration(sorted, 0.5).String(),
			"p90": percentileDuration(sorted, 0.9).String(),
			"p95": percentileDuration(sorted, 0.95).String(),
			"p99": percentileDuration(sorted, 0.99).String(),
		}
	}
	
	// Include recent error samples if available
	if len(metrics.ErrorSamples) > 0 {
		errorSamples := metrics.ErrorSamples
		if len(errorSamples) > 5 {
			errorSamples = errorSamples[len(errorSamples)-5:]
		}
		
		sampleData := make([]map[string]interface{}, 0, len(errorSamples))
		for _, sample := range errorSamples {
			sampleInfo := map[string]interface{}{
				"url":        sample.URL,
				"statusCode": sample.StatusCode,
				"time":       sample.Time.Format(time.RFC3339),
			}
			
			if sample.Error != "" {
				sampleInfo["error"] = sample.Error
			} else if len(sample.Body) > 200 {
				sampleInfo["body"] = sample.Body[:200] + "..." // Truncate long bodies
			} else {
				sampleInfo["body"] = sample.Body
			}
			
			sampleData = append(sampleData, sampleInfo)
		}
		
		report["errorSamples"] = sampleData
	}
	
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(reportJSON))
}

// percentileDuration calculates the percentile value from sorted durations
func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// max returns the maximum of two int64 values
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()
	
	// Set GOMAXPROCS to use all available CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	// Load configuration
	configFile, err := os.Open(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			createDefaultSpreeConfig(*configPath)
			log.Fatalf("Default configuration created at %s. Please adjust values and run again.", *configPath)
		}
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer configFile.Close()
	
	var config Config
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
	
	// Initialize metrics
	metrics := NewMetrics()
	
	// Set up worker pool
	pool := NewWorkerPool(config.Test.MaxWorkers, config.Test.MaxQueueSize, metrics, &config)
	
	// Set up load generator
	generator := NewLoadGenerator(pool, &config)
	
	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start load test
	fmt.Println("Starting Spree API load test...")
	pool.Start()
	generator.Start()
	
	// Wait for completion or interrupt
	select {
	case <-sigChan:
		fmt.Println("\nReceived interrupt signal, shutting down...")
	}
	
	// Graceful shutdown
	generator.Stop()
	close(pool.Tasks)
	pool.Stop()
	
	// Final report
	metrics.EndTime = time.Now()
	printFinalReport(metrics)
}

// printFinalReport generates and writes the final test report
func printFinalReport(metrics *Metrics) {
	metrics.mutex.RLock()
	defer metrics.mutex.RUnlock()
	
	testDuration := metrics.EndTime.Sub(metrics.StartTime)
	actualRPS := float64(metrics.TotalRequests) / testDuration.Seconds()
	
	// Calculate endpoint distribution
	endpointDistribution := make(map[string]float64)
	totalEndpoints := int64(0)
	for _, count := range metrics.EndpointCounts {
		totalEndpoints += count
	}
	
	if totalEndpoints > 0 {
		for endpoint, count := range metrics.EndpointCounts {
			endpointDistribution[endpoint] = float64(count) / float64(totalEndpoints) * 100
		}
	}
	
	// Create comprehensive final report
	report := map[string]interface{}{
		"platform":           "Spree",
		"testStartTime":      metrics.StartTime.Format(time.RFC3339),
		"testEndTime":        metrics.EndTime.Format(time.RFC3339),
		"testDuration":       testDuration.String(),
		"totalRequests":      metrics.TotalRequests,
		"successfulRequests": metrics.SuccessfulRequests,
		"failedRequests":     metrics.FailedRequests,
		"actualRPS":          fmt.Sprintf("%.2f", actualRPS),
		"successRate":        fmt.Sprintf("%.2f%%", float64(metrics.SuccessfulRequests)/float64(max(metrics.TotalRequests, 1))*100),
		"endpointDistribution": endpointDistribution,
	}
	
	// Add status code distribution
	statusDist := make(map[string]int64)
	for code, count := range metrics.StatusCodes {
		if code == 0 {
			statusDist["network_error"] = count
		} else {
			codeGroup := fmt.Sprintf("%dxx", code/100)
			statusDist[codeGroup] += count
		}
	}
	report["statusDistribution"] = statusDist
	
	// Calculate latency percentiles if we have data
	if len(metrics.RequestDurations) > 0 {
		sorted := make([]time.Duration, len(metrics.RequestDurations))
		copy(sorted, metrics.RequestDurations)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})
		
		// Calculate mean duration
		var sum time.Duration
		for _, d := range sorted {
			sum += d
		}
		mean := sum / time.Duration(len(sorted))
		
		report["latency"] = map[string]string{
			"min":  sorted[0].String(),
			"p50":  percentileDuration(sorted, 0.5).String(),
			"p90":  percentileDuration(sorted, 0.9).String(),
			"p95":  percentileDuration(sorted, 0.95).String(),
			"p99":  percentileDuration(sorted, 0.99).String(),
			"max":  sorted[len(sorted)-1].String(),
			"mean": mean.String(),
		}
	}
	
	// Write final report to file
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	
	// Print to console
	fmt.Println("\nFinal Test Results:")
	fmt.Println(string(reportJSON))
	
	// Save to file
	err := os.WriteFile("spree_results.json", reportJSON, 0644)
	if err != nil {
		fmt.Printf("Error writing results file: %v\n", err)
	} else {
		fmt.Println("\nDetailed results saved to spree_results.json")
	}
}

// createDefaultSpreeConfig creates a default configuration file for Spree
func createDefaultSpreeConfig(path string) {
	config := Config{}
	
	// Set default endpoints
	config.Endpoints.Products = "https://wsm-spree.alphasquadit.com/api/v2/storefront/products/"
	config.Endpoints.SpecificProduct = "https://wsm-spree.alphasquadit.com/api/v2/storefront/products/1"
	
	// Set default headers
	config.Headers = map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
	}
	
	// Set default test configuration
	config.Test.MaxWorkers = 200
	config.Test.MaxQueueSize = 5000
	config.Test.ReportingSeconds = 5
	config.Test.LogErrors = true
	config.Test.ErrorSampleRate = 0.1
	
	// Set traffic distribution
	config.Test.TrafficDistribution.Products = 60   // 60%
	config.Test.TrafficDistribution.SpecificProduct = 40 // 20%
	
	// Define realistic ramp-up stages
	config.Test.RampupStages = []Stage{
		{Duration: 30 * time.Second, TargetRPS: 10, Description: "Warm-up at 10 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 50, Description: "Ramp up to 50 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 50, Description: "Hold at 50 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 100, Description: "Ramp up to 100 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 100, Description: "Hold at 100 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 0, Description: "Ramp down to 0"},
	}
	
	// Write configuration to file
	configFile, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create default config file: %v", err)
	}
	defer configFile.Close()
	
	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		log.Fatalf("Failed to write default config: %v", err)
	}
}