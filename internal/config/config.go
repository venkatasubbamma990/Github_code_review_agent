package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                string
	GinMode             string
	LogLevel            string
	LogFormat           string
	LLMProvider         string
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	LLMJSONMode         bool
	GitHubToken         string
	GitHubWebhookSecret string
	GitHubPostComments  bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	provider := getEnv("LLM_PROVIDER", "groq")
	baseURL, model := llmDefaults(provider)

	return &Config{
		Port:                getEnv("PORT", "8080"),
		GinMode:             getEnv("GIN_MODE", "debug"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LogFormat:           getEnv("LOG_FORMAT", "console"),
		LLMProvider:         provider,
		LLMAPIKey:           os.Getenv("LLM_API_KEY"),
		LLMBaseURL:          getEnv("LLM_BASE_URL", baseURL),
		LLMModel:            getEnv("LLM_MODEL", model),
		LLMJSONMode:         getEnvBool("LLM_JSON_MODE", true),
		GitHubToken:         os.Getenv("GITHUB_TOKEN"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitHubPostComments:  getEnvBool("GITHUB_POST_COMMENTS", true),
	}, nil
}

func llmDefaults(provider string) (baseURL, model string) {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1", "gpt-4o-mini"
	case "groq":
		return "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile"
	default:
		return "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile"
	}
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
