# Temporal Cloud Proxy

A Temporal Cloud proxy that handles Temporal namespace authentication, payload encryption/decryption, and client/worker authentication for multiple workloads with different configurations.

## Key Features

- **Multi-workload Support** - Handle multiple Temporal workload configurations through a single proxy instance
- **Payload Encryption/Decryption** - AWS KMS and GCP KMS support with intelligent caching for performance
- **Temporal Cloud Namespace Authentication** - Support for mTLS and API keys
- **Worker Authentication** - Support for JWT (with JWKS) and SPIFFE/SPIRE
- **Observability** - Built-in Prometheus metrics, Grafana dashboards, and structured logging

## Quick Start

### Prerequisites

- Go 1.24 or later
- AWS account with KMS permissions (for AWS KMS encryption)
- GCP account with KMS permissions (for GCP KMS encryption)
- Temporal Cloud account

### Installation

1. Clone the repository:

```bash
git clone <repository-url>
cd temporal-cloud-proxy
```

2. Build the binary:

```bash
make build
```

3. Copy and configure the sample config:

```bash
cp config.yaml.sample config.yaml
# Edit config.yaml with your settings
```

4. Run the proxy:

```bash
./tclp --config config.yaml
```

## Configuration Reference

The proxy is configured via a YAML file. Here's the basic structure:

```yaml
server:
  port: 7233 # Proxy server port
  host: "0.0.0.0" # Bind address

metrics:
  port: 9090 # Prometheus metrics port

encryption:
  caching:
    max_cache: 100 # Maximum cached encryption keys
    max_age: "10m" # Key cache TTL
    max_usage: 100 # Maximum key usage count

workloads:
  - workload_id: "my-workload"
    temporal_cloud:
      namespace: "my-namespace.my-account"
      host_port: "my-namespace.my-account.tmprl.cloud:7233"
      authentication:
        # Choose one authentication method
        tls:
          cert_file: "/path/to/tls.crt"
          key_file: "/path/to/tls.key"
        # OR
        api_key:
          value: "your-api-key"
          # OR env: "API_KEY_ENV_VAR"
    encryption:
      type: "aws-kms"
      config:
        key-id: "arn:aws:kms:region:account:key/key-id"
    authentication:
      type: "jwt"
      config:
        jwks-url: "https://your-auth-provider/.well-known/keys"
        audiences: ["temporal_cloud_proxy"]
```

### Multiple Workloads Example

```yaml
workloads:
  - workload_id: "production"
    temporal_cloud:
      namespace: "prod.company"
      host_port: "prod.company.tmprl.cloud:7233"
      authentication:
        api_key:
          env: "PROD_API_KEY"
    encryption:
      type: "aws-kms"
      config:
        key-id: "arn:aws:kms:us-east-1:123456789:key/prod-key"
    authentication:
      type: "spiffe"
      config:
        trust_domain: "spiffe://company.com/"
        endpoint: "unix:///tmp/spire-agent/public/api.sock"

  - workload_id: "staging"
    temporal_cloud:
      namespace: "staging.company"
      host_port: "staging.company.tmprl.cloud:7233"
      authentication:
        tls:
          cert_file: "/certs/staging.crt"
          key_file: "/certs/staging.key"
    encryption:
      type: "gcp-kms"
      config:
        key-name: "projects/my-project/locations/us-central1/keyRings/staging/cryptoKeys/temporal"
```

## Temporal Cloud Namespace Authentication Methods

### mTLS Authentication

```yaml
temporal_cloud:
  authentication:
    tls:
      cert_file: "/path/to/client.crt"
      key_file: "/path/to/client.key"
```

### API Key Authentication

```yaml
temporal_cloud:
  authentication:
    api_key:
      value: "your-api-key"
      # OR use environment variable
      env: "TEMPORAL_API_KEY"
```

## Worker to Proxy Authentication Methods

### JWT Authentication

```yaml
authentication:
  type: "jwt"
  config:
    jwks-url: "https://auth.company.com/.well-known/keys"
    audiences: ["temporal_cloud_proxy"]
```

### SPIFFE Authentication

```yaml
authentication:
  type: "spiffe"
  config:
    trust_domain: "spiffe://company.com/"
    endpoint: "unix:///tmp/spire-agent/public/api.sock"
    audiences: ["temporal_cloud_proxy"]
```

## Encryption & Security

### AWS KMS Configuration

1. Create a KMS key in AWS:

```bash
aws kms create-key --description "Temporal Cloud Proxy Encryption Key"
```

2. Configure the proxy:

```yaml
encryption:
  type: "aws-kms"
  config:
    key-id: "arn:aws:kms:region:account-id:key/key-id"
```

3. Ensure AWS credentials are configured:

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=your_region
```

### GCP KMS Configuration

1. Create a KMS key in GCP:

```bash
gcloud kms keyrings create temporal-proxy --location=global
gcloud kms keys create encryption-key --location=global --keyring=temporal-proxy --purpose=encryption
```

2. Configure the proxy:

```yaml
encryption:
  type: "gcp-kms"
  config:
    key-name: "projects/PROJECT_ID/locations/global/keyRings/temporal-proxy/cryptoKeys/encryption-key"
```

### Encryption Caching

The proxy includes intelligent caching to optimize encryption performance:

```yaml
encryption:
  caching:
    max_cache: 100 # Maximum number of cached keys
    max_age: "10m" # Maximum age of cached keys
    max_usage: 100 # Maximum usage count per key
```

## Monitoring & Observability

### Prometheus Metrics

The proxy exposes metrics on the configured metrics port (default: 9090):

- `proxy_request_total` - Total number of proxy requests
- `proxy_request_errors` - Number of failed requests
- `proxy_request_success` - Number of successful requests
- `proxy_latency` - Request latency histogram
- Encryption/decryption metrics per workload
- Authentication success/failure rates

### Grafana Dashboard

A pre-configured Grafana dashboard is available at `dashboards/grafana-dashboard.json`. Import this dashboard to visualize:

- Request throughput and error rates
- Authentication success rates
- Encryption performance metrics
- Per-workload statistics

### Logging

Configure log levels using the `--log-level` flag:

```bash
./tclp --config config.yaml --log-level debug
```

Available levels: `debug`, `info`, `warn`, `error`

## Development & Testing

### Building

```bash
# Build the binary
make build

# Clean build artifacts
make clean

# Build and test
make all
```

### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage
make test-coverage

# Run race condition tests
make test-race

# Run benchmarks
make benchmark
```

## API Reference

### gRPC Interface

The proxy implements the standard Temporal gRPC interface with additional metadata requirements:

#### Required Metadata

- `workload-id`: Identifies which workload configuration to use
- `authorization`: Authentication token (when worker authentication is enabled)

#### Example Client Usage

```go
conn, err := grpc.Dial("proxy:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
if err != nil {
    log.Fatal(err)
}

// Add required metadata
ctx := metadata.AppendToOutgoingContext(context.Background(),
    "workload-id", "my-workload",
    "authorization", "Bearer "+token,
)

// Use the connection with Temporal client
client := temporal.NewClient(temporal.Options{
    HostPort: "proxy:7233",
    // ... other options
})
```


### Debug Logging

Enable debug logging for detailed troubleshooting:

```bash
./tclp --config config.yaml --log-level debug
```
