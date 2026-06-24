package main

import (
	"log"

	"codereviewagent/internal/config"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/handler"
	"codereviewagent/internal/reviewer"
	"codereviewagent/internal/server"
	"codereviewagent/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	llmReviewer := reviewer.NewLLMReviewer(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	gh := ghclient.NewClient(cfg.GitHubToken)
	reviewSvc := service.NewReviewService(llmReviewer, gh, cfg.GitHubPostComments)
	reviewHandler := handler.NewReviewHandler(reviewSvc, cfg.GitHubWebhookSecret)

	srv := server.New(reviewHandler, cfg.GinMode)

	addr := ":" + cfg.Port
	log.Printf("Code Review Agent starting on %s", addr)
	if err := srv.Engine().Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
