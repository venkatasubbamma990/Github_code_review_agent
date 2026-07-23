package main

import (
	"context"

	"codereviewagent/internal/config"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/handler"
	"codereviewagent/internal/logger"
	"codereviewagent/internal/queue"
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
		zap.String("mode", "multi-agent"),
		zap.String("redis", cfg.RedisAddr),
	)

	multiAgentReviewer := reviewer.NewMultiAgentReviewer(
		cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMJSONMode,
		cfg.MaxChunkBytes, cfg.GosecPath, cfg.SemgrepPath, log,
	)
	gh := ghclient.NewClient(cfg.GitHubToken, log)
	q := queue.NewClient(cfg.RedisAddr, log)
	defer func() { _ = q.Close() }()

	if cfg.GitHubWebhookSecret != "" && !q.Enabled() {
		log.Fatal("GITHUB_WEBHOOK_SECRET is set but REDIS_ADDR is empty — Redis is required for reliable webhook processing")
	}

	reviewSvc := service.NewReviewService(multiAgentReviewer, gh, q, cfg.GitHubPostComments, cfg.GitHubPostChecks, cfg.MaxRepoFiles, log)
	reviewHandler := handler.NewReviewHandler(reviewSvc, cfg.GitHubWebhookSecret, log)

	if q.Enabled() {
		worker := queue.NewWorker(cfg.RedisAddr, queue.TaskHandler{
			OnPRReview: func(ctx context.Context, p queue.PRReviewPayload) ([]byte, error) {
				return reviewSvc.ProcessPRReviewTask(ctx, p.Owner, p.Repo, p.PRNumber)
			},
			OnRepoReview: func(ctx context.Context, p queue.RepoReviewPayload) ([]byte, error) {
				return reviewSvc.ProcessRepoReviewTask(ctx, p.Owner, p.Repo, p.Branch, p.MaxFiles)
			},
			Log: log,
		}, log)
		go func() {
			if err := worker.Run(); err != nil {
				log.Error("async worker stopped", zap.Error(err))
			}
		}()
		log.Info("async worker started", zap.String("redis", cfg.RedisAddr))
	} else {
		log.Warn("redis not configured — webhooks and async reviews will return 503 until REDIS_ADDR is set")
	}

	srv := server.New(reviewHandler, cfg.GinMode, log)

	addr := ":" + cfg.Port
	log.Info("server listening", zap.String("addr", addr))
	if err := srv.Engine().Run(addr); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
