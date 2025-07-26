package config

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"os"
)

const (
	ConfigPathFlag    = "config"
	DefaultConfigPath = "config.yaml"
	LogLevelFlag      = "level"

	AwsRegionEnvVar  = "AWS_REGION"
	DefaultAwsRegion = "us-west-2"

	GcpRegionEnvVar  = "GCP_REGION"
	DefaultGcpRegion = "us-central1"
)

type (
	ConfigProvider interface {
		GetProxyConfig() ProxyConfig
	}

	ProxyConfig struct {
		Server     ServerConfig           `yaml:"server"`
		Metrics    MetricsConfig          `yaml:"metrics"`
		Encryption GlobalEncryptionConfig `yaml:"encryption"`
		Workloads  []WorkloadConfig       `yaml:"workloads"`
	}

	ServerConfig struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	}

	MetricsConfig struct {
		Port int `yaml:"port"`
	}

	GlobalEncryptionConfig struct {
		Caching CachingConfig `yaml:"caching"`
	}

	CachingConfig struct {
		MaxCache int    `yaml:"max_cache,omitempty"`
		MaxAge   string `yaml:"max_age,omitempty"`
		MaxUsage int    `yaml:"max_usage,omitempty"`
	}

	WorkloadConfig struct {
		WorkloadId     string              `yaml:"workload_id"`
		TemporalCloud  TemporalCloudConfig `yaml:"temporal_cloud"`
		Encryption     *EncryptionConfig   `yaml:"encryption,omitempty"`
		Authentication *AuthConfig         `yaml:"authentication,omitempty"`
	}

	TemporalCloudConfig struct {
		Namespace      string             `yaml:"namespace"`
		HostPort       string             `yaml:"host_port"`
		Authentication TemporalAuthConfig `yaml:"authentication"`
	}

	TemporalAuthConfig struct {
		TLS    *TLSConfig            `yaml:"tls,omitempty"`
		ApiKey *TemporalApiKeyConfig `yaml:"api_key,omitempty"`
	}

	TemporalApiKeyConfig struct {
		Value  string `yaml:"value,omitempty"`
		EnvVar string `yaml:"env,omitempty"`
	}

	TLSConfig struct {
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
	}

	AuthConfig struct {
		Type   string                 `yaml:"type"`
		Config map[string]interface{} `yaml:"config"`
	}

	EncryptionConfig struct {
		Type   string                 `yaml:"type"`
		Config map[string]interface{} `yaml:"config"`
	}

	cliConfigProvider struct {
		ctx         *cli.Context
		proxyConfig ProxyConfig
	}
)

func newConfigProvider(ctx *cli.Context) (ConfigProvider, error) {
	proxyConfig, err := LoadConfig(ctx.String(ConfigPathFlag))
	if err != nil {
		return nil, err
	}

	return &cliConfigProvider{
		ctx:         ctx,
		proxyConfig: proxyConfig,
	}, nil
}

func (c *cliConfigProvider) GetProxyConfig() ProxyConfig {
	return c.proxyConfig
}

func LoadConfig(configFilePath string) (ProxyConfig, error) {
	var config ProxyConfig

	configFile, err := os.ReadFile(configFilePath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if err = yaml.Unmarshal(configFile, &config); err != nil {
		return config, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	if err = config.Validate(); err != nil {
		return config, fmt.Errorf("failed to validate config: %w", err)
	}

	return config, nil
}

func (p *ProxyConfig) Validate() error {
	var errs []error

	errs = append(errs, p.Server.Validate()...)
	errs = append(errs, p.Metrics.Validate()...)
	errs = append(errs, p.Encryption.Validate()...)

	workloadIds := make(map[string]bool)
	for _, workload := range p.Workloads {
		if _, exists := workloadIds[workload.WorkloadId]; exists {
			errs = append(errs, fmt.Errorf("workload already exists: %s", workload.WorkloadId))
		}
		errs = append(errs, workload.Validate()...)
		workloadIds[workload.WorkloadId] = true
	}

	return errors.Join(errs...)
}

func (s *ServerConfig) Validate() []error {
	var errs []error

	if s.Port <= 0 || s.Port > 65535 {
		errs = append(errs, fmt.Errorf("invalid server port: %d", s.Port))
	}

	return errs
}

func (m *MetricsConfig) Validate() []error {
	var errs []error

	if m.Port <= 0 || m.Port > 65535 {
		errs = append(errs, fmt.Errorf("invalid metrics server port: %d", m.Port))
	}

	return errs
}

func (g *GlobalEncryptionConfig) Validate() []error {
	var errs []error

	if g.Caching.MaxCache < 0 {
		errs = append(errs, fmt.Errorf("encryption max_cache must be >= 0: %d", g.Caching.MaxCache))
	}

	if g.Caching.MaxUsage < 0 {
		errs = append(errs, fmt.Errorf("encryption max_usage must be >= 0: %d", g.Caching.MaxUsage))
	}

	return errs
}

func (w *WorkloadConfig) Validate() []error {
	var errs []error

	if w.WorkloadId == "" {
		errs = append(errs, fmt.Errorf("workload_id is required"))
	}

	if w.TemporalCloud.Namespace == "" {
		errs = append(errs, fmt.Errorf("temporal cloud namespace must not be blank: %s", w.WorkloadId))
	}

	if w.TemporalCloud.HostPort == "" {
		errs = append(errs, fmt.Errorf("temporal cloud hostport must not be blank: %s", w.WorkloadId))
	}

	if w.TemporalCloud.Authentication.ApiKey != nil && w.TemporalCloud.Authentication.TLS != nil {
		errs = append(errs, fmt.Errorf(
			"cannot have both api key and mtls authentication configured on a single workload: %s", w.WorkloadId,
		))
	}

	return errs
}
