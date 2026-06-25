package main

import (
	"codereviewagent/internal/config"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/handler"
	"codereviewagent/internal/logger"
	"codereviewagent/internal/reviewer"
	"codereviewagent/internal/server"
	"codereviewagent/internal/service"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	development := cfg.GinMode == "debug"
	log, err := logger.New(cfg.LogLevel, cfg.LogFormat, development)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting code review agent",
		zap.String("port", cfg.Port),
		zap.String("llm_provider", cfg.LLMProvider),
		zap.String("llm_model", cfg.LLMModel),
		zap.String("log_level", cfg.LogLevel),
	)

	llmReviewer := reviewer.NewLLMReviewer(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMJSONMode, log)
	gh := ghclient.NewClient(cfg.GitHubToken, log)
	reviewSvc := service.NewReviewService(llmReviewer, gh, cfg.GitHubPostComments, log)
	reviewHandler := handler.NewReviewHandler(reviewSvc, cfg.GitHubWebhookSecret, log)

	srv := server.New(reviewHandler, cfg.GinMode, log)

	addr := ":" + cfg.Port
	log.Info("server listening", zap.String("addr", addr))
	if err := srv.Engine().Run(addr); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
