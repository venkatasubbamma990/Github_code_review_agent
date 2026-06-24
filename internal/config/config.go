package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                string
	GinMode             string
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	GitHubToken         string
	GitHubWebhookSecret string
	GitHubPostComments  bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
		Port:                getEnv("PORT", "8080"),
		GinMode:             getEnv("GIN_MODE", "debug"),
		LLMAPIKey:           os.Getenv("LLM_API_KEY"),
		LLMBaseURL:          getEnv("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMModel:            getEnv("LLM_MODEL", "gpt-4o-mini"),
		GitHubToken:         os.Getenv("GITHUB_TOKEN"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitHubPostComments:  getEnvBool("GITHUB_POST_COMMENTS", true),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
