package utils

type Config struct {
	Server  ServerConfig   `yaml:"server"`
	Targets []TargetConfig `yaml:"targets"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
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
