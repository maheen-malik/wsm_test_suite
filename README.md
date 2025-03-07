# Extreme Load Testing Tool

This Golang application is designed to perform extreme load testing, capable of generating up to 4.8 million requests per second. It was created to match the functionality of the provided K6 script.

## Features

- Configurable ramp-up stages with precise control of request rates
- Worker pool architecture for high concurrency
- Efficient memory management with customizable metrics sampling
- Runtime statistics reporting with configurable intervals
- Even distribution of requests across multiple endpoints
- Graceful shutdown on interrupt signals

## Requirements

- Go 1.18 or higher
- Sufficient system resources (CPU, memory, network bandwidth)
- Proper system tuning for high-volume network traffic

## System Tuning

To achieve very high RPS rates, you'll need to tune your operating system. Here are some recommended settings:

### Linux

Add the following to `/etc/sysctl.conf` and run `sudo sysctl -p`:

```
# Increase system file descriptor limit
fs.file-max = 2097152

# Increase TCP max connection parameters
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 262144
net.ipv4.tcp_max_syn_backlog = 262144

# Increase local port range
net.ipv4.ip_local_port_range = 1024 65535

# Reuse TIME-WAIT sockets
net.ipv4.tcp_tw_reuse = 1

# TCP memory usage
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
```

Also, make sure to increase user limits in `/etc/security/limits.conf`:

```
* soft nofile 1048576
* hard nofile 1048576
```

## Usage

1. Build the application:
   ```
   go build -o loadtester
   ```

2. Run with the default configuration:
   ```
   ./loadtester
   ```

   Or specify a custom configuration file:
   ```
   ./loadtester -config custom-config.json
   ```

## Configuration

The application uses a JSON configuration file with the following structure:

```json
{
  "Endpoints": {
    "Products": "https://example.com/products",
    "Categories": "https://example.com/categories",
    "SpecificCategory": "https://example.com/specific-category"
  },
  "APIKey": "your-api-key",
  "Test": {
    "MaxWorkers": 100000,
    "MaxQueueSize": 1000000,
    "ReportingSeconds": 30,
    "RampupStages": [
      {
        "Duration": 180000000000,
        "TargetRPS": 10000,
        "Description": "Warm-up at 10k RPS"
      },
      ...
    ]
  }
}
```

### Running on Multiple Machines

For extremely high load testing, you may need to run the application on multiple machines. The load will be distributed across all instances. Make sure each machine is properly tuned for high network throughput.

## Testing Strategy

1. Start with the small-scale configuration (`config-small.json`) to verify the application works correctly.
2. Monitor system resource usage and adjust the configuration as needed.
3. Gradually increase the load to ensure your systems can handle it.
4. For the full 4.8M RPS test, you will likely need multiple high-spec machines.

## Monitoring

The application outputs periodic JSON reports with metrics including:
- Total requests sent
- Success/failure counts and rates
- Actual RPS achieved
- Latency percentiles (p50, p90, p95, p99)

## Distributed Execution

For achieving the highest load rates (such as 4.8M RPS), you'll need to run the tool on multiple machines. Here's a strategy:

1. Distribute the same application to multiple servers.
2. Create separate configuration files for each server with appropriate RPS targets.
3. Start the tests simultaneously on all machines.
4. Aggregate the results manually.

A rough formula to determine how many machines you need:
- Each modern server with good tuning can typically handle 100K-300K RPS.
- For 4.8M RPS, you might need 16-48 servers depending on their specifications.

## Important Notes

- Running at extremely high RPS can cause issues with network equipment, cloud providers, and target systems.
- Always coordinate with the operations team and monitoring systems before running extreme load tests.
- Be prepared to quickly terminate the test if it causes problems.
- Consider implementing a back-off mechanism if the target system starts failing.