package config

import (
	"os"
)

type Config struct {
	TelegramToken string
	StagingPath   string
	Sources       []string
	DebugMode     bool
}

func LoadFromEnv() *Config {
	cfg := &Config{
		TelegramToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		StagingPath:   getEnv("STAGING_PATH", "/data/staging"),
		Sources:       []string{"qobuz", "deezer"},
		DebugMode:     getEnv("DEBUG", "false") == "true",
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
