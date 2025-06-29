package utils

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type ConfigManager struct {
	configPath   string
	config       *Config
	lastLoadTime time.Time
	mu           sync.RWMutex
}

func NewConfigManager(configPath string) (*ConfigManager, error) {
	cm := &ConfigManager{
		configPath: configPath,
	}

	if err := cm.loadConfig(); err != nil {
		return nil, err
	}

	return cm, nil
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

func (cm *ConfigManager) Close() error {
	return nil
}

func (cm *ConfigManager) loadConfig() error {
	configFile, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err = yaml.Unmarshal(configFile, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	cm.mu.Lock()
	cm.config = &cfg
	cm.lastLoadTime = time.Now()
	cm.mu.Unlock()

	return nil
}
