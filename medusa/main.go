package main

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"encoding/json"
)

type Config struct {
	Endpoints struct {
		Products string
		Categories string
		SpecificCategory string
	}
	APIKey string
	Test struct {
		MaxWorkers int
		MaxQueueSize int
		RampupStages []Stage
		ReportingSeconds int
	}
}


type Stage struct {
	Duration time.Duration
	TargetRPS int64
	Description string
}

type Metrics struct {
	StartTime time.Time
	EndTime time.Time
	TotalRequests int64
	SuccessfulRequests int64
	FailedRequests int64
	RequestDurations []time.Duration
	mutex sync.Mutex
}

// Add a result to the metrics
func (m *Metrics) AddResult(duration time.Duration, success bool) {
	atomic.AddInt64(&m.TotalRequests, 1)
	if success {
		atomic.AddInt64(&m.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&m.FailedRequests, 1)
	}
	
	// Only store a sample of durations to avoid memory issues
	if rand.Float64() < 0.01 { // Store only 1% of durations
		m.mutex.Lock()
		m.RequestDurations = append(m.RequestDurations, duration)
		m.mutex.Unlock()
	}
}

// Calculate statistics for the report
func (m *Metrics) CalculateStats() map[string]interface{} {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	testDuration := time.Since(m.StartTime)
	actualRPS := float64(m.TotalRequests) / testDuration.Seconds()
	
	// Calculate percentiles
	var p50, p90, p95, p99 time.Duration
	if len(m.RequestDurations) > 0 {
		// Sort durations for percentile calculation
		durations := make([]time.Duration, len(m.RequestDurations))
		copy(durations, m.RequestDurations)
		
		// Quick sort implementation with custom comparator
		// This is much faster than using sort.Slice for large slices
		sortDurations(durations)
		
		p50 = percentileDuration(durations, 0.5)
		p90 = percentileDuration(durations, 0.9)
		p95 = percentileDuration(durations, 0.95)
		p99 = percentileDuration(durations, 0.99)
	}
	
	return map[string]interface{}{
		"totalRequests":      m.TotalRequests,
		"successfulRequests": m.SuccessfulRequests,
		"failedRequests":     m.FailedRequests,
		"testDuration":       testDuration.String(),
		"actualRPS":          fmt.Sprintf("%.2f", actualRPS),
		"successRate":        fmt.Sprintf("%.2f%%", float64(m.SuccessfulRequests)/float64(m.TotalRequests)*100),
		"latency": map[string]string{
			"p50": p50.String(),
			"p90": p90.String(),
			"p95": p95.String(),
			"p99": p99.String(),
		},
	}
}

// percentileDuration calculates the percentile value from sorted durations
func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Floor(percentile * float64(len(sorted))))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// sortDurations sorts the durations slice in place
func sortDurations(durations []time.Duration) {
	if len(durations) <= 1 {
		return
	}
	quickSortDurations(durations, 0, len(durations)-1)
}

// quickSortDurations implements quicksort for duration slices
func quickSortDurations(durations []time.Duration, low, high int) {
	if low < high {
		pivot := partitionDurations(durations, low, high)
		quickSortDurations(durations, low, pivot-1)
		quickSortDurations(durations, pivot+1, high)
	}
}

// partitionDurations partitions the slice for quicksort
func partitionDurations(durations []time.Duration, low, high int) int {
	pivot := durations[high]
	i := low - 1
	
	for j := low; j < high; j++ {
		if durations[j] <= pivot {
			i++
			durations[i], durations[j] = durations[j], durations[i]
		}
	}
	
	durations[i+1], durations[high] = durations[high], durations[i+1]
	return i + 1
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
	CurrentRate *atomic.Int64 // Current RPS target being achieved
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers, queueSize int, metrics *Metrics) *WorkerPool {
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
		Timeout:   5 * time.Second,
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
		p.Metrics.AddResult(0, false)
		return
	}
	
	// Add headers
	for key, value := range task.Headers {
		req.Header.Set(key, value)
	}
	
	start := time.Now()
	resp, err := p.HTTPClient.Do(req)
	duration := time.Since(start)
	
	success := err == nil && resp != nil && resp.StatusCode == http.StatusOK
	
	if resp != nil {
		// Discard response body but ensure connection is closed properly
		resp.Body.Close()
	}
	
	p.Metrics.AddResult(duration, success)
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
				stats := g.Pool.Metrics.CalculateStats()
				statsJSON, _ := json.MarshalIndent(stats, "", "  ")
				fmt.Println(string(statsJSON))
			case <-g.StopChan:
				return
			}
		}
	}()
	
	// Calculate the initial interval between requests
	interval := time.Second / time.Duration(currentTargetRPS)
	log.Print("Interval: ", interval)
	
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
					
					// Recalculate interval between requests
					if currentTargetRPS > 0 {
						interval = time.Second / time.Duration(currentTargetRPS)
					}
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

// generateTask creates a new HTTP request task
func (g *LoadGenerator) generateTask() Task {
	// Distribute traffic across endpoints
	var url, taskType string
	switch rand.Intn(3) {
	case 0:
		url = g.Config.Endpoints.Products
		taskType = "products"
	case 1:
		url = g.Config.Endpoints.Categories
		taskType = "categories"
	default:
		url = g.Config.Endpoints.SpecificCategory
		taskType = "specific_category"
	}
	
	headers := map[string]string{
		"x-publishable-api-key": g.Config.APIKey,
		"Accept":                "application/json",
		"Content-Type":          "application/json",
	}
	
	return Task{
		URL:     url,
		Headers: headers,
		Method:  "GET",
		Type:    taskType,
	}
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
		// If config file doesn't exist, create a default one
		if os.IsNotExist(err) {
			createDefaultConfig(*configPath)
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
	metrics := &Metrics{
		StartTime: time.Now(),
	}
	
	// Set up worker pool
	pool := NewWorkerPool(config.Test.MaxWorkers, config.Test.MaxQueueSize, metrics)
	
	// Set up load generator
	generator := NewLoadGenerator(pool, &config)
	
	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start load test
	fmt.Println("Starting extreme load test...")
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
	finalStats := metrics.CalculateStats()
	finalStatsJSON, _ := json.MarshalIndent(finalStats, "", "  ")
	fmt.Println("\nFinal Test Results:")
	fmt.Println(string(finalStatsJSON))
}

// createDefaultConfig creates a default configuration file
func createDefaultConfig(path string) {
	config := Config{}
	
	// Set default endpoints matching the K6 script
	config.Endpoints.Products = "https://wsm-medusa.alphasquadit.com/store/products"
	config.Endpoints.Categories = "https://wsm-medusa.alphasquadit.com/store/product-categories/"
	config.Endpoints.SpecificCategory = "https://wsm-medusa.alphasquadit.com/store/product-categories/pcat_01JNGVR2XQS0BWM1VNFBVH8KJ9"
	
	// Set default API key
	config.APIKey = "pk_05c09b4f04e7185405f50dee26b6846b278aa7bd7b4b42b1fe6d42e5fe9ee390"
	
	// Set default test configuration
	config.Test.MaxWorkers = 100000
	config.Test.MaxQueueSize = 1000000
	config.Test.ReportingSeconds = 30
	
	// Define ramp-up stages matching the K6 script
	config.Test.RampupStages = []Stage{
		{Duration: 3 * time.Minute, TargetRPS: 10000, Description: "Warm-up at 10k RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 100000, Description: "Ramp up to 100k RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 100000, Description: "Hold at 100k RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 500000, Description: "Ramp up to 500k RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 500000, Description: "Hold at 500k RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 1000000, Description: "Ramp up to 1M RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 1000000, Description: "Hold at 1M RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 2400000, Description: "Ramp up to 2.4M RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 2400000, Description: "Hold at 2.4M RPS"},
		{Duration: 10 * time.Minute, TargetRPS: 4800000, Description: "Ramp up to 4.8M RPS"},
		{Duration: 10 * time.Minute, TargetRPS: 4800000, Description: "Stay at 4.8M RPS"},
		{Duration: 5 * time.Minute, TargetRPS: 0, Description: "Ramp down to 0"},
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