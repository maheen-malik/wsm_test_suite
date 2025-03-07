package main

import (
	"bytes"
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
	// GraphQL endpoint
	GraphQLURL string

	// GraphQL queries
	Queries struct {
		Products        string
		Categories      string
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
	}
}

// Stage represents a load testing stage
type Stage struct {
	Duration    time.Duration
	TargetRPS   int64
	Description string
}

// GraphQLRequest represents a GraphQL query or mutation
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL API response
type GraphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// ErrorResponse tracks details about failed requests
type ErrorResponse struct {
	Query       string
	StatusCode  int
	Body        string
	GraphQLErrs []string
	Time        time.Time
	Error       string // If error occurred before getting a response
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
	OperationCounts    map[string]int64
	ErrorSamples       []ErrorResponse
	mutex              sync.RWMutex
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:       time.Now(),
		StatusCodes:     make(map[int]int64),
		OperationCounts: make(map[string]int64),
		ErrorSamples:    make([]ErrorResponse, 0, 100),
	}
}

// AddResult adds a result to the metrics
func (m *Metrics) AddResult(duration time.Duration, operation string, statusCode int, errResp *ErrorResponse) {
	atomic.AddInt64(&m.TotalRequests, 1)

	m.mutex.Lock()
	m.OperationCounts[operation]++
	m.StatusCodes[statusCode]++
	m.mutex.Unlock()

	if statusCode >= 200 && statusCode < 300 && errResp == nil {
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

// Calculate percentile from sorted durations
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

// Task represents a single GraphQL request to be executed
type Task struct {
	Query     string
	Variables map[string]interface{}
	Operation string // For metrics tracking
}

// WorkerPool for handling concurrent requests
type WorkerPool struct {
	Tasks       chan Task
	Workers     int
	StopChan    chan struct{}
	WaitGroup   sync.WaitGroup
	HTTPClient  *http.Client
	GraphQLURL  string
	Headers     map[string]string
	Metrics     *Metrics
	CurrentRate *atomic.Int64
	Config      *Config
}

// NewWorkerPool creates a new worker pool for Saleor GraphQL requests
func NewWorkerPool(workers, queueSize int, graphqlURL string, headers map[string]string, metrics *Metrics, config *Config) *WorkerPool {
	// Create an optimized HTTP transport
	transport := &http.Transport{
		MaxIdleConns:        workers,
		MaxIdleConnsPerHost: workers,
		MaxConnsPerHost:     workers,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	currentRate := &atomic.Int64{}
	currentRate.Store(0)

	return &WorkerPool{
		Tasks:       make(chan Task, queueSize),
		Workers:     workers,
		StopChan:    make(chan struct{}),
		HTTPClient:  client,
		GraphQLURL:  graphqlURL,
		Headers:     headers,
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

// worker processes GraphQL tasks from the queue
func (p *WorkerPool) worker() {
	defer p.WaitGroup.Done()

	for {
		select {
		case task, ok := <-p.Tasks:
			if !ok {
				return
			}
			p.executeGraphQLTask(task)
		case <-p.StopChan:
			return
		}
	}
}

// executeGraphQLTask performs the GraphQL request
func (p *WorkerPool) executeGraphQLTask(task Task) {
	// Prepare GraphQL request
	graphqlReq := GraphQLRequest{
		Query:     task.Query,
		Variables: task.Variables,
	}

	reqBody, err := json.Marshal(graphqlReq)
	if err != nil {
		errResp := &ErrorResponse{
			Query: task.Query,
			Time:  time.Now(),
			Error: fmt.Sprintf("request marshaling error: %v", err),
		}
		p.Metrics.AddResult(0, task.Operation, 0, errResp)
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", p.GraphQLURL, bytes.NewBuffer(reqBody))
	if err != nil {
		errResp := &ErrorResponse{
			Query: task.Query,
			Time:  time.Now(),
			Error: fmt.Sprintf("request creation error: %v", err),
		}
		p.Metrics.AddResult(0, task.Operation, 0, errResp)
		return
	}

	// Add headers
	for key, value := range p.Headers {
		req.Header.Set(key, value)
	}

	// Execute request with timing
	start := time.Now()
	resp, err := p.HTTPClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		errResp := &ErrorResponse{
			Query: task.Query,
			Time:  time.Now(),
			Error: fmt.Sprintf("request error: %v", err),
		}
		p.Metrics.AddResult(duration, task.Operation, 0, errResp)
		return
	}

	defer resp.Body.Close()

	// Process response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errResp := &ErrorResponse{
			Query:      task.Query,
			StatusCode: resp.StatusCode,
			Time:       time.Now(),
			Error:      fmt.Sprintf("error reading response: %v", err),
		}
		p.Metrics.AddResult(duration, task.Operation, resp.StatusCode, errResp)
		return
	}

	// Parse GraphQL response
	var graphqlResp GraphQLResponse
	err = json.Unmarshal(body, &graphqlResp)

	var errResp *ErrorResponse
	if err != nil {
		// JSON parsing error
		errResp = &ErrorResponse{
			Query:      task.Query,
			StatusCode: resp.StatusCode,
			Time:       time.Now(),
			Error:      fmt.Sprintf("error parsing response: %v", err),
		}
	} else if resp.StatusCode >= 400 {
		// HTTP error
		errResp = &ErrorResponse{
			Query:      task.Query,
			StatusCode: resp.StatusCode,
			Body:       string(body),
			Time:       time.Now(),
		}
	} else if graphqlResp.Errors != nil && len(graphqlResp.Errors) > 0 {
		// GraphQL error
		var graphqlErrors []string
		for _, e := range graphqlResp.Errors {
			graphqlErrors = append(graphqlErrors, e.Message)
		}

		errResp = &ErrorResponse{
			Query:       task.Query,
			StatusCode:  resp.StatusCode,
			Body:        string(body),
			GraphQLErrs: graphqlErrors,
			Time:        time.Now(),
		}
	}

	// Only create error sample if enabled and within sample rate
	if errResp != nil && p.Config.Test.LogErrors && rand.Float64() <= p.Config.Test.ErrorSampleRate {
		p.Metrics.AddResult(duration, task.Operation, resp.StatusCode, errResp)
	} else {
		p.Metrics.AddResult(duration, task.Operation, resp.StatusCode, nil)
	}
}

// LoadGenerator controls the rate of GraphQL request generation
type LoadGenerator struct {
	Pool      *WorkerPool
	Config    *Config
	StopChan  chan struct{}
	WaitGroup sync.WaitGroup
}

// NewLoadGenerator creates a new GraphQL load generator
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

// generateGraphQLTask creates a new GraphQL request task with even distribution
func (g *LoadGenerator) generateGraphQLTask() Task {
	// Distribute traffic across query types
	var query string
	var operation string

	rand := rand.Float64()
	if rand < 0.333 {
		query = g.Config.Queries.Products
		operation = "products"
	} else if rand < 0.666 {
		query = g.Config.Queries.Categories
		operation = "categories"
	} else {
		query = g.Config.Queries.SpecificProduct
		operation = "specific_product"
	}

	return Task{
		Query:     query,
		Variables: nil, // No variables for these basic queries
		Operation: operation,
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
				printGraphQLReport(g.Pool.Metrics, currentTargetRPS)
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
				task := g.generateGraphQLTask()

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

// printGraphQLReport generates and prints a report of current GraphQL metrics
func printGraphQLReport(metrics *Metrics, targetRPS int64) {
	metrics.mutex.RLock()
	defer metrics.mutex.RUnlock()

	testDuration := time.Since(metrics.StartTime)
	actualRPS := float64(metrics.TotalRequests) / testDuration.Seconds()

	// Calculate operation distribution
	operationDistribution := make(map[string]float64)
	totalOps := int64(0)
	for _, count := range metrics.OperationCounts {
		totalOps += count
	}

	if totalOps > 0 {
		for op, count := range metrics.OperationCounts {
			operationDistribution[op] = float64(count) / float64(totalOps) * 100
		}
	}

	// Create basic report
	report := map[string]interface{}{
		"totalRequests":         metrics.TotalRequests,
		"successfulRequests":    metrics.SuccessfulRequests,
		"failedRequests":        metrics.FailedRequests,
		"testDuration":          testDuration.String(),
		"actualRPS":             fmt.Sprintf("%.2f", actualRPS),
		"targetRPS":             targetRPS,
		"successRate":           fmt.Sprintf("%.2f%%", float64(metrics.SuccessfulRequests)/float64(max(metrics.TotalRequests, 1))*100),
		"statusCodes":           metrics.StatusCodes,
		"operationDistribution": operationDistribution,
	}

	// Calculate latency percentiles if we have data
	if len(metrics.RequestDurations) > 0 {
		// Sort the durations for percentile calculation
		sorted := make([]time.Duration, len(metrics.RequestDurations))
		copy(sorted, metrics.RequestDurations)
		sort.Sort(durationSlice(sorted))

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
				"operation":  sample.Query,
				"statusCode": sample.StatusCode,
				"time":       sample.Time.Format(time.RFC3339),
			}

			if len(sample.GraphQLErrs) > 0 {
				sampleInfo["graphqlErrors"] = sample.GraphQLErrs
			}

			if sample.Error != "" {
				sampleInfo["error"] = sample.Error
			}

			sampleData = append(sampleData, sampleInfo)
		}

		report["errorSamples"] = sampleData
	}

	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(reportJSON))
}

// Helper for sorting durations
type durationSlice []time.Duration

func (s durationSlice) Len() int           { return len(s) }
func (s durationSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s durationSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

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
			createDefaultSaleorConfig(*configPath)
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

	// Set up worker pool for GraphQL
	pool := NewWorkerPool(
		config.Test.MaxWorkers,
		config.Test.MaxQueueSize,
		config.GraphQLURL,
		config.Headers,
		metrics,
		&config,
	)

	// Set up load generator
	generator := NewLoadGenerator(pool, &config)

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start load test
	fmt.Println("Starting Saleor GraphQL load test...")
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

	// Create comprehensive final report
	report := map[string]interface{}{
		"platform":           "Saleor",
		"testStartTime":      metrics.StartTime.Format(time.RFC3339),
		"testEndTime":        metrics.EndTime.Format(time.RFC3339),
		"testDuration":       testDuration.String(),
		"totalRequests":      metrics.TotalRequests,
		"successfulRequests": metrics.SuccessfulRequests,
		"failedRequests":     metrics.FailedRequests,
		"actualRPS":          fmt.Sprintf("%.2f", actualRPS),
		"successRate":        fmt.Sprintf("%.2f%%", float64(metrics.SuccessfulRequests)/float64(max(metrics.TotalRequests, 1))*100),
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

	// Add operation distribution
	opDist := make(map[string]float64)
	totalOps := int64(0)
	for _, count := range metrics.OperationCounts {
		totalOps += count
	}
	if totalOps > 0 {
		for op, count := range metrics.OperationCounts {
			opDist[op] = float64(count) / float64(totalOps) * 100
		}
	}
	report["operationDistribution"] = opDist

	// Calculate latency percentiles if we have data
	if len(metrics.RequestDurations) > 0 {
		sorted := make([]time.Duration, len(metrics.RequestDurations))
		copy(sorted, metrics.RequestDurations)
		sort.Sort(durationSlice(sorted))

		report["latency"] = map[string]string{
			"min":  sorted[0].String(),
			"p50":  percentileDuration(sorted, 0.5).String(),
			"p90":  percentileDuration(sorted, 0.9).String(),
			"p95":  percentileDuration(sorted, 0.95).String(),
			"p99":  percentileDuration(sorted, 0.99).String(),
			"max":  sorted[len(sorted)-1].String(),
			"mean": calculateMeanDuration(sorted).String(),
		}
	}

	// Write final report to file
	reportJSON, _ := json.MarshalIndent(report, "", "  ")

	// Print to console
	fmt.Println("\nFinal Test Results:")
	fmt.Println(string(reportJSON))

	// Save to file
	err := os.WriteFile("saleor_results.json", reportJSON, 0644)
	if err != nil {
		fmt.Printf("Error writing results file: %v\n", err)
	} else {
		fmt.Println("\nDetailed results saved to saleor_results.json")
	}
}

// calculateMeanDuration calculates the mean of a slice of durations
func calculateMeanDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}

	return sum / time.Duration(len(durations))
}

// createDefaultSaleorConfig creates a default configuration file for Saleor
func createDefaultSaleorConfig(path string) {
	config := Config{}

	// Set default GraphQL endpoint
	config.GraphQLURL = "https://wsm-saleor.alphasquadit.com/graphql/"

	// Set default headers
	config.Headers = map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	// Set default queries
	config.Queries.Products = `{
		products(first: 10, channel: "default-channel") {
			edges {
				node {
					id
					name
				}
			}
		}
	}`

	config.Queries.Categories = `{
		categories(first: 10) {
			edges {
				node {
					id
					name
				}
			}
		}
	}`

	config.Queries.SpecificProduct = `{
		product(id: "UHJvZHVjdDo3Mg==", channel: "default-channel") {
			id
			name
			description
			pricing {
				priceRange {
					start {
						gross {
							amount
							currency
						}
					}
				}
			}
		}
	}`

	// Set default test configuration
	config.Test.MaxWorkers = 200
	config.Test.MaxQueueSize = 5000
	config.Test.ReportingSeconds = 5
	config.Test.LogErrors = true
	config.Test.ErrorSampleRate = 0.1

	// Define realistic ramp-up stages
	config.Test.RampupStages = []Stage{
		{Duration: 30 * time.Second, TargetRPS: 10, Description: "Warm-up at 10 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 50, Description: "Raise to 50 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 50, Description: "Ramp up to 50 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 50, Description: "Hold at 50 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 100, Description: "Ramp up to 100 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 100, Description: "Hold at 100 RPS"},
		{Duration: 30 * time.Second, TargetRPS: 0, Description: "Ramp down to 0"},
	}
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
