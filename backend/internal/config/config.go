package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Engine   EngineConfig
	Auth     AuthConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port        int
	CORSOrigins []string
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver string
	DSN    string
}

// EngineConfig holds calculation engine settings.
type EngineConfig struct {
	MaxWorkers            int
	IntermediatePrecision int32
	OutputPrecision       int32
	ParallelThreshold     int
	CacheSize             int
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret   string
	TokenExpiry time.Duration
}

// Load reads configuration from environment variables, applying sensible defaults.
func Load() (*Config, error) {
	port := envInt("SERVER_PORT", 8080)
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("SERVER_PORT must be between 1 and 65535, got %d", port)
	}

	corsOrigins := envStringSlice("SERVER_CORS_ORIGINS", []string{"http://localhost:5173"})

	driver := envString("DB_DRIVER", "sqlite")
	switch driver {
	case "sqlite", "postgres", "mysql":
	default:
		return nil, fmt.Errorf("DB_DRIVER must be sqlite, postgres, or mysql, got %q", driver)
	}

	dsn := envString("DB_DSN", "file:formula.db?_journal=WAL&_foreign_keys=on")

	maxWorkers := envInt("ENGINE_MAX_WORKERS", 0)
	if maxWorkers == 0 {
		maxWorkers = runtime.NumCPU()
	}
	if maxWorkers < 0 {
		return nil, fmt.Errorf("ENGINE_MAX_WORKERS must be non-negative, got %d", maxWorkers)
	}

	intermediatePrecision := int32(envInt("ENGINE_INTERMEDIATE_PRECISION", 28))
	outputPrecision := int32(envInt("ENGINE_OUTPUT_PRECISION", 18))
	parallelThreshold := envInt("ENGINE_PARALLEL_THRESHOLD", 8)
	cacheSize := envInt("ENGINE_CACHE_SIZE", 1000)

	jwtSecret := envString("AUTH_JWT_SECRET", "")
	tokenExpiry := envDuration("AUTH_TOKEN_EXPIRY", 24*time.Hour)

	cfg := &Config{
		Server: ServerConfig{
			Port:        port,
			CORSOrigins: corsOrigins,
		},
		Database: DatabaseConfig{
			Driver: driver,
			DSN:    dsn,
		},
		Engine: EngineConfig{
			MaxWorkers:            maxWorkers,
			IntermediatePrecision: intermediatePrecision,
			OutputPrecision:       outputPrecision,
			ParallelThreshold:     parallelThreshold,
			CacheSize:             cacheSize,
		},
		Auth: AuthConfig{
			JWTSecret:   jwtSecret,
			TokenExpiry: tokenExpiry,
		},
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func envStringSlice(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}
