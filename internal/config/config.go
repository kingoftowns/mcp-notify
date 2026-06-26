package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port             string `env:"PORT"                envDefault:"8080"`
	GmailUsername    string `env:"GMAIL_USERNAME,required"`
	GmailAppPassword string `env:"GMAIL_APP_PASSWORD,required"`
	BearerToken      string `env:"BEARER_TOKEN"`
	NotifyRecipient  string `env:"NOTIFY_RECIPIENT,required"`
}

// Load parses environment variables into a Config struct using caarlos0/env.
func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}
