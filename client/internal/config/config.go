package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerURL             string
	RunnerSlug            string
	RunnerName            string
	RunnerTags            []string
	MaxConcurrency        int
	GPUCapable            bool
	LogBatchInterval      time.Duration
	MetricSampleInterval  time.Duration
	MetricBatchSize       int
	HeartbeatInterval     time.Duration
	APIToken              string
	TLSSkipVerify         bool
	CACertPath            string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	tagsStr := getEnvOrDefault("CLIENT_RUNNER_TAGS", "")
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	cfg := &Config{
		ServerURL:             getEnvOrDefault("CLIENT_SERVER_URL", "http://localhost:80"),
		RunnerSlug:            os.Getenv("CLIENT_RUNNER_SLUG"),
		RunnerName:            getEnvOrDefault("CLIENT_RUNNER_NAME", ""),
		RunnerTags:            tags,
		MaxConcurrency:        getEnvAsIntOrDefault("CLIENT_MAX_CONCURRENCY", 4),
		GPUCapable:            getEnvAsBoolOrDefault("CLIENT_GPU_CAPABLE", false),
		LogBatchInterval:      time.Duration(getEnvAsIntOrDefault("CLIENT_LOG_BATCH_INTERVAL_MS", 500)) * time.Millisecond,
		MetricSampleInterval:  time.Duration(getEnvAsIntOrDefault("CLIENT_METRIC_SAMPLE_INTERVAL_MS", 1000)) * time.Millisecond,
		MetricBatchSize:       getEnvAsIntOrDefault("CLIENT_METRIC_BATCH_SIZE", 50),
		HeartbeatInterval:     time.Duration(getEnvAsIntOrDefault("CLIENT_HEARTBEAT_INTERVAL_SECS", 30)) * time.Second,
		APIToken:              getEnvOrDefault("CLIENT_API_TOKEN", "secret"),
		TLSSkipVerify:         getEnvAsBoolOrDefault("CLIENT_TLS_SKIP_VERIFY", false),
		CACertPath:            getEnvOrDefault("CLIENT_CA_CERT_PATH", ""),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("CLIENT_SERVER_URL is required")
	}
	if c.RunnerSlug == "" {
		return fmt.Errorf("CLIENT_RUNNER_SLUG is required")
	}
	if c.RunnerName == "" {
		c.RunnerName = c.RunnerSlug
	}
	if c.MaxConcurrency < 1 {
		return fmt.Errorf("CLIENT_MAX_CONCURRENCY must be at least 1")
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
