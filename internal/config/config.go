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
	RedisAddr           string
	GosecPath           string
	SemgrepPath         string
	MaxChunkBytes       int
	MaxRepoFiles        int
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
		RedisAddr:           getEnv("REDIS_ADDR", ""),
		GosecPath:           getEnv("GOSEC_PATH", "gosec"),
		SemgrepPath:         getEnv("SEMGREP_PATH", "semgrep"),
		MaxChunkBytes:       getEnvInt("MAX_CHUNK_BYTES", 50000),
		MaxRepoFiles:        getEnvInt("MAX_REPO_FILES", 20),
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

func getEnvInt(key string, fallback int) int {
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
