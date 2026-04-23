package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Workers  WorkersConfig
	Traefik  TraefikConfig
}

type ServerConfig struct {
	Port string
	Env  string
}

type PostgresConfig struct {
	Host                 string
	Port                 string
	Database             string
	User                 string
	Password             string
	MaxConns             int
	QueuePollInterval    time.Duration
}

type WorkersConfig struct {
	ReaperInterval             time.Duration
	RunnerOrphanThreshold      time.Duration
	CommandOrphanDeadline      time.Duration
	RetentionInterval          time.Duration
	RollupInterval             time.Duration
	LogRetentionDaysSystem     int
	LogRetentionDaysCommand    int
	LogRetentionSizeMBCommand  int
	MetricRetentionDaysRaw     int
	MetricRetentionDays1M      int
	MetricRetentionDays5M      int
}

type TraefikConfig struct {
	APIToken string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvOrDefault("SERVER_PORT", "8080"),
			Env:  getEnvOrDefault("SERVER_ENV", "development"),
		},
		Postgres: PostgresConfig{
			Host:                 getEnvOrDefault("POSTGRES_HOST", "postgres"),
			Port:                 getEnvOrDefault("POSTGRES_PORT", "5432"),
			Database:             getEnvOrDefault("POSTGRES_DB", "runnerdb"),
			User:                 getEnvOrDefault("POSTGRES_USER", "runner"),
			Password:             getEnvOrDefault("POSTGRES_PASSWORD", "secret"),
			MaxConns:             getEnvAsIntOrDefault("POSTGRES_MAX_CONNS", 20),
			QueuePollInterval:    time.Duration(getEnvAsIntOrDefault("POSTGRES_QUEUE_POLL_INTERVAL_MS", 500)) * time.Millisecond,
		},
		Workers: WorkersConfig{
			ReaperInterval:             time.Duration(getEnvAsIntOrDefault("REAPER_INTERVAL_SECS", 30)) * time.Second,
			RunnerOrphanThreshold:      time.Duration(getEnvAsIntOrDefault("RUNNER_ORPHAN_THRESHOLD_SECS", 90)) * time.Second,
			CommandOrphanDeadline:      time.Duration(getEnvAsIntOrDefault("COMMAND_ORPHAN_DEADLINE_SECS", 300)) * time.Second,
			RetentionInterval:          time.Duration(getEnvAsIntOrDefault("RETENTION_INTERVAL_SECS", 3600)) * time.Second,
			RollupInterval:             time.Duration(getEnvAsIntOrDefault("ROLLUP_INTERVAL_SECS", 300)) * time.Second,
			LogRetentionDaysSystem:     getEnvAsIntOrDefault("LOG_RETENTION_DAYS_SYSTEM", 7),
			LogRetentionDaysCommand:    getEnvAsIntOrDefault("LOG_RETENTION_DAYS_COMMAND", 30),
			LogRetentionSizeMBCommand:  getEnvAsIntOrDefault("LOG_RETENTION_SIZE_MB_COMMAND", 100),
			MetricRetentionDaysRaw:     getEnvAsIntOrDefault("METRIC_RETENTION_DAYS_RAW", 1),
			MetricRetentionDays1M:      getEnvAsIntOrDefault("METRIC_RETENTION_DAYS_1M", 7),
			MetricRetentionDays5M:      getEnvAsIntOrDefault("METRIC_RETENTION_DAYS_5M", 30),
		},
		Traefik: TraefikConfig{
			APIToken: getEnvOrDefault("TRAEFIK_API_TOKEN", "secret"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Postgres.Host == "" {
		return fmt.Errorf("POSTGRES_HOST is required")
	}
	if c.Postgres.Database == "" {
		return fmt.Errorf("POSTGRES_DB is required")
	}
	if c.Postgres.User == "" {
		return fmt.Errorf("POSTGRES_USER is required")
	}
	if c.Postgres.Password == "" {
		return fmt.Errorf("POSTGRES_PASSWORD is required")
	}
	return nil
}

func (c *PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Database)
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
