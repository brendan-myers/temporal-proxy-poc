package utils

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Encryption EncryptionConfig `yaml:"encryption"`
	Targets    []TargetConfig   `yaml:"targets"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type MetricsConfig struct {
	Port int `yaml:"port"`
}

type EncryptionConfig struct {
	Caching CachingConfig `yaml:"caching"`
}

type CachingConfig struct {
	MaxCache int    `yaml:"max_cache,omitempty"`
	MaxAge   string `yaml:"max_age,omitempty"`
	MaxUsage int    `yaml:"max_usage,omitempty"`
}

type TargetConfig struct {
	Source         string      `yaml:"source"`
	Target         string      `yaml:"target"`
	TLS            TLSConfig   `yaml:"tls"`
	EncryptionKey  string      `yaml:"encryption_key"`
	Namespace      string      `yaml:"namespace"`
	Authentication *AuthConfig `yaml:"authentication,omitempty"`
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type AuthConfig struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}
