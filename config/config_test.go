package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			configYAML: `
server:
  port: 7233
  host: "0.0.0.0"
metrics:
  port: 9090
encryption:
  caching:
    max_cache: 100
    max_age: "10m"
    max_usage: 100
workloads:
  - workload_id: "test-workload"
    temporal_cloud:
      namespace: "test.namespace"
      host_port: "test.namespace.tmprl.cloud:7233"
      authentication:
        api_key:
          value: "test-key"
`,
			expectError: false,
		},
		{
			name: "invalid yaml",
			configYAML: `
server:
  port: 7233
  host: "0.0.0.0"
invalid_yaml: [
`,
			expectError: true,
			errorMsg:    "failed to unmarshal config file",
		},
		{
			name: "validation failure - invalid port",
			configYAML: `
server:
  port: 70000
  host: "0.0.0.0"
metrics:
  port: 9090
workloads: []
`,
			expectError: true,
			errorMsg:    "failed to validate config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpFile, err := os.CreateTemp("", "config-*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.configYAML)
			require.NoError(t, err)
			tmpFile.Close()

			// Test LoadConfig
			config, err := LoadConfig(tmpFile.Name())

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, config)
			}
		})
	}
}

func TestProxyConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ProxyConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ProxyConfig{
				Server: ServerConfig{
					Port: 7233,
					Host: "0.0.0.0",
				},
				Metrics: MetricsConfig{
					Port: 9090,
				},
				Encryption: GlobalEncryptionConfig{
					Caching: CachingConfig{
						MaxCache: 100,
						MaxAge:   "10m",
						MaxUsage: 100,
					},
				},
				Workloads: []WorkloadConfig{
					{
						WorkloadId: "test-workload",
						TemporalCloud: TemporalCloudConfig{
							Namespace: "test.namespace",
							HostPort:  "test.namespace.tmprl.cloud:7233",
							Authentication: TemporalAuthConfig{
								ApiKey: &TemporalApiKeyConfig{
									Value: "test-key",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate workload IDs",
			config: ProxyConfig{
				Server: ServerConfig{Port: 7233, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: 9090},
				Workloads: []WorkloadConfig{
					{
						WorkloadId: "duplicate-id",
						TemporalCloud: TemporalCloudConfig{
							Namespace: "test1.namespace",
							HostPort:  "test1.namespace.tmprl.cloud:7233",
							Authentication: TemporalAuthConfig{
								ApiKey: &TemporalApiKeyConfig{Value: "key1"},
							},
						},
					},
					{
						WorkloadId: "duplicate-id",
						TemporalCloud: TemporalCloudConfig{
							Namespace: "test2.namespace",
							HostPort:  "test2.namespace.tmprl.cloud:7233",
							Authentication: TemporalAuthConfig{
								ApiKey: &TemporalApiKeyConfig{Value: "key2"},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "workload already exists: duplicate-id",
		},
		{
			name: "invalid server port - too high",
			config: ProxyConfig{
				Server:  ServerConfig{Port: 70000, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: 9090},
			},
			wantErr: true,
			errMsg:  "invalid server port: 70000",
		},
		{
			name: "invalid server port - zero",
			config: ProxyConfig{
				Server:  ServerConfig{Port: 0, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: 9090},
			},
			wantErr: true,
			errMsg:  "invalid server port: 0",
		},
		{
			name: "invalid metrics port - negative",
			config: ProxyConfig{
				Server:  ServerConfig{Port: 7233, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: -1},
			},
			wantErr: true,
			errMsg:  "invalid metrics server port: -1",
		},
		{
			name: "negative encryption max_cache",
			config: ProxyConfig{
				Server:  ServerConfig{Port: 7233, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: 9090},
				Encryption: GlobalEncryptionConfig{
					Caching: CachingConfig{MaxCache: -1},
				},
			},
			wantErr: true,
			errMsg:  "encryption max_cache must be >= 0: -1",
		},
		{
			name: "negative encryption max_usage",
			config: ProxyConfig{
				Server:  ServerConfig{Port: 7233, Host: "0.0.0.0"},
				Metrics: MetricsConfig{Port: 9090},
				Encryption: GlobalEncryptionConfig{
					Caching: CachingConfig{MaxUsage: -5},
				},
			},
			wantErr: true,
			errMsg:  "encryption max_usage must be >= 0: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkloadConfig_Validate(t *testing.T) {
	tests := []struct {
		name     string
		workload WorkloadConfig
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid workload with API key",
			workload: WorkloadConfig{
				WorkloadId: "test-workload",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "test.namespace",
					HostPort:  "test.namespace.tmprl.cloud:7233",
					Authentication: TemporalAuthConfig{
						ApiKey: &TemporalApiKeyConfig{
							Value: "test-key",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid workload with TLS",
			workload: WorkloadConfig{
				WorkloadId: "test-workload",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "test.namespace",
					HostPort:  "test.namespace.tmprl.cloud:7233",
					Authentication: TemporalAuthConfig{
						TLS: &TLSConfig{
							CertFile: "/path/to/cert.pem",
							KeyFile:  "/path/to/key.pem",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing workload_id",
			workload: WorkloadConfig{
				WorkloadId: "",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "test.namespace",
					HostPort:  "test.namespace.tmprl.cloud:7233",
				},
			},
			wantErr: true,
			errMsg:  "workload_id is required",
		},
		{
			name: "missing namespace",
			workload: WorkloadConfig{
				WorkloadId: "test-workload",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "",
					HostPort:  "test.namespace.tmprl.cloud:7233",
				},
			},
			wantErr: true,
			errMsg:  "temporal cloud namespace must not be blank: test-workload",
		},
		{
			name: "missing host_port",
			workload: WorkloadConfig{
				WorkloadId: "test-workload",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "test.namespace",
					HostPort:  "",
				},
			},
			wantErr: true,
			errMsg:  "temporal cloud hostport must not be blank: test-workload",
		},
		{
			name: "both API key and TLS configured",
			workload: WorkloadConfig{
				WorkloadId: "test-workload",
				TemporalCloud: TemporalCloudConfig{
					Namespace: "test.namespace",
					HostPort:  "test.namespace.tmprl.cloud:7233",
					Authentication: TemporalAuthConfig{
						ApiKey: &TemporalApiKeyConfig{
							Value: "test-key",
						},
						TLS: &TLSConfig{
							CertFile: "/path/to/cert.pem",
							KeyFile:  "/path/to/key.pem",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "cannot have both api key and mtls authentication configured on a single workload: test-workload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.workload.Validate()
			
			if tt.wantErr {
				assert.NotEmpty(t, errs)
				if tt.errMsg != "" {
					found := false
					for _, err := range errs {
						if assert.Contains(t, err.Error(), tt.errMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error message not found in: %v", errs)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config ServerConfig
		wantErr bool
		errMsg string
	}{
		{
			name:    "valid port",
			config:  ServerConfig{Port: 8080, Host: "localhost"},
			wantErr: false,
		},
		{
			name:    "port too low",
			config:  ServerConfig{Port: 0, Host: "localhost"},
			wantErr: true,
			errMsg:  "invalid server port: 0",
		},
		{
			name:    "port too high",
			config:  ServerConfig{Port: 70000, Host: "localhost"},
			wantErr: true,
			errMsg:  "invalid server port: 70000",
		},
		{
			name:    "negative port",
			config:  ServerConfig{Port: -1, Host: "localhost"},
			wantErr: true,
			errMsg:  "invalid server port: -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.config.Validate()

			if tt.wantErr {
				assert.NotEmpty(t, errs)
				if tt.errMsg != "" {
					assert.Contains(t, errs[0].Error(), tt.errMsg)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestMetricsConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config MetricsConfig
		wantErr bool
		errMsg string
	}{
		{
			name:    "valid port",
			config:  MetricsConfig{Port: 9090},
			wantErr: false,
		},
		{
			name:    "port too low",
			config:  MetricsConfig{Port: 0},
			wantErr: true,
			errMsg:  "invalid metrics server port: 0",
		},
		{
			name:    "port too high",
			config:  MetricsConfig{Port: 70000},
			wantErr: true,
			errMsg:  "invalid metrics server port: 70000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.config.Validate()

			if tt.wantErr {
				assert.NotEmpty(t, errs)
				if tt.errMsg != "" {
					assert.Contains(t, errs[0].Error(), tt.errMsg)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestGlobalEncryptionConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config GlobalEncryptionConfig
		wantErr bool
		errMsg string
	}{
		{
			name: "valid config",
			config: GlobalEncryptionConfig{
				Caching: CachingConfig{
					MaxCache: 100,
					MaxUsage: 50,
				},
			},
			wantErr: false,
		},
		{
			name: "zero values are valid",
			config: GlobalEncryptionConfig{
				Caching: CachingConfig{
					MaxCache: 0,
					MaxUsage: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "negative max_cache",
			config: GlobalEncryptionConfig{
				Caching: CachingConfig{
					MaxCache: -1,
					MaxUsage: 50,
				},
			},
			wantErr: true,
			errMsg:  "encryption max_cache must be >= 0: -1",
		},
		{
			name: "negative max_usage",
			config: GlobalEncryptionConfig{
				Caching: CachingConfig{
					MaxCache: 100,
					MaxUsage: -5,
				},
			},
			wantErr: true,
			errMsg:  "encryption max_usage must be >= 0: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.config.Validate()

			if tt.wantErr {
				assert.NotEmpty(t, errs)
				if tt.errMsg != "" {
					assert.Contains(t, errs[0].Error(), tt.errMsg)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}
