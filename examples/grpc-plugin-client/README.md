# GRPC Plugin Client Example

This is a complete example demonstrating how to create a client that communicates with the GRPC Plugin Monitor in the Node Problem Detector.

## Features

- Secure TLS communication over Unix domain sockets
- Support for both permanent (node condition changes) and temporary (event-only) problem reporting
- Health check functionality
- Continuous monitoring mode
- Comprehensive command-line options

## Building

```bash
cd examples/grpc-plugin-client
go build -o grpc-client .
```

## Usage Examples

### Basic Problem Reporting

Report a single permanent problem:

```bash
./grpc-client \
  -source="my-application" \
  -type="permanent" \
  -condition="CustomApplicationProblem" \
  -reason="ApplicationDown" \
  -message="My application is not responding" \
  -status="warning"
```

Report a temporary problem (event only):

```bash
./grpc-client \
  -source="monitoring-agent" \
  -type="temporary" \
  -reason="HighMemoryUsage" \
  -message="Memory usage exceeded 90%" \
  -status="warning" \
  -severity="warn"
```

### Health Check

Check if the GRPC server is healthy:

```bash
./grpc-client -health=true
```

### Continuous Monitoring

Run in continuous mode (reports problems every 30 seconds):

```bash
./grpc-client \
  -continuous=true \
  -source="continuous-monitor" \
  -type="permanent" \
  -condition="DatabaseConnectivityProblem" \
  -reason="DatabaseDown" \
  -message="Cannot connect to database"
```

### TLS Configuration

By default, TLS is enabled. You can configure certificate paths:

```bash
./grpc-client \
  -tls=true \
  -cert="/path/to/client.crt" \
  -key="/path/to/client.key" \
  -ca="/path/to/ca.crt" \
  -source="secure-client"
```

For testing purposes only, you can skip TLS verification:

```bash
./grpc-client -insecure=true -source="test-client"
```

Or disable TLS entirely:

```bash
./grpc-client -tls=false -source="insecure-client"
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-socket` | `/var/run/node-problem-detector/grpc-plugin-monitor.sock` | Path to the unix socket |
| `-tls` | `true` | Enable TLS |
| `-cert` | `/etc/node-problem-detector/certs/client.crt` | Client certificate file |
| `-key` | `/etc/node-problem-detector/certs/client.key` | Client private key file |
| `-ca` | `/etc/node-problem-detector/certs/ca.crt` | CA certificate file |
| `-source` | `example-client` | Source identifier for this client |
| `-type` | `permanent` | Problem type (`permanent` or `temporary`) |
| `-condition` | `CustomApplicationProblem` | Condition name for permanent problems |
| `-reason` | `ApplicationDown` | Problem reason |
| `-message` | `Custom application is not responding` | Problem message |
| `-status` | `warning` | Problem status (`ok`, `warning`, `unknown`) |
| `-severity` | `warn` | Problem severity (`info`, `warn`, `error`) |
| `-health` | `false` | Perform health check instead of reporting problems |
| `-continuous` | `false` | Run in continuous mode |
| `-insecure` | `false` | Skip TLS verification (testing only) |

## Problem Types

### Permanent Problems

Permanent problems change the node condition status. They should be used for persistent issues that affect the node's ability to schedule workloads.

- **Type**: `permanent`
- **Condition**: Must be specified and match a condition defined in the server configuration
- **Effect**: Changes node condition status and generates events

Example conditions:
- `CustomApplicationProblem`
- `DatabaseConnectivityProblem`
- `ExternalServiceProblem`

### Temporary Problems

Temporary problems generate events only and don't change node conditions. They should be used for transient issues or alerts.

- **Type**: `temporary`
- **Condition**: Not used
- **Effect**: Generates events only

## Integration with Kubernetes

When used with Node Problem Detector in a Kubernetes cluster:

1. **Node Conditions**: Permanent problems with `warning` status will set the corresponding node condition to `True`
2. **Node Events**: All problems generate events visible with `kubectl get events`
3. **Metrics**: If metrics reporting is enabled, problems are exposed as Prometheus metrics
4. **Node Taints**: Depending on NPD configuration, conditions may result in node taints

## Certificate Setup

For production use, you'll need to set up proper certificates:

1. **CA Certificate**: Root certificate authority
2. **Server Certificate**: For the GRPC server (signed by CA)
3. **Client Certificate**: For the GRPC client (signed by CA)

Example using OpenSSL:

```bash
# Generate CA key and certificate
openssl genrsa -out ca.key 4096
openssl req -new -x509 -key ca.key -sha256 -subj "/C=US/ST=CA/O=NPD/CN=CA" -days 3650 -out ca.crt

# Generate server key and certificate
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr -subj "/C=US/ST=CA/O=NPD/CN=localhost"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -sha256

# Generate client key and certificate
openssl genrsa -out client.key 4096
openssl req -new -key client.key -out client.csr -subj "/C=US/ST=CA/O=NPD/CN=client"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt -days 365 -sha256
```

## Error Handling

The client handles various error scenarios:

- **Connection failures**: Retries with exponential backoff
- **TLS verification failures**: Clear error messages about certificate issues
- **Server unavailable**: Graceful handling when server is down
- **Invalid parameters**: Validation of command-line arguments

## Best Practices

1. **Use permanent problems sparingly**: Only for issues that should affect node scheduling
2. **Provide meaningful reasons and messages**: Help operators understand the problem
3. **Use appropriate severity levels**: Match severity to the impact of the problem
4. **Include useful metadata**: Add context that helps with debugging
5. **Monitor client health**: Ensure your monitoring client is itself monitored
6. **Use secure communication**: Always enable TLS in production environments