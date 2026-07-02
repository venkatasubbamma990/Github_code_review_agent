package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

const (
	TaskReviewPR   = "review:pr"
	TaskReviewRepo = "review:repo"
)

type PRReviewPayload struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
}

type RepoReviewPayload struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	MaxFiles int    `json:"max_files"`
}

type JobStatus struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	State   string          `json:"state"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Created time.Time       `json:"created"`
}

type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
	log       *zap.Logger
	enabled   bool
}

func NewClient(redisAddr string, log *zap.Logger) *Client {
	if redisAddr == "" {
		return &Client{log: log.Named("queue"), enabled: false}
	}

	opt := asynq.RedisClientOpt{Addr: redisAddr}
	return &Client{
		client:    asynq.NewClient(opt),
		inspector: asynq.NewInspector(opt),
		log:       log.Named("queue"),
		enabled:   true,
	}
}

func (c *Client) Enabled() bool {
	return c.enabled
}

func (c *Client) Close() error {
	if !c.enabled {
		return nil
	}
	return c.client.Close()
}

func (c *Client) EnqueuePRReview(owner, repo string, prNumber int) (string, error) {
	payload, err := json.Marshal(PRReviewPayload{Owner: owner, Repo: repo, PRNumber: prNumber})
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TaskReviewPR, payload)
	info, err := c.client.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(30*time.Minute))
	if err != nil {
		return "", err
	}
	c.log.Info("PR review enqueued", zap.String("task_id", info.ID))
	return info.ID, nil
}

func (c *Client) EnqueueRepoReview(owner, repo, branch string, maxFiles int) (string, error) {
	payload, err := json.Marshal(RepoReviewPayload{Owner: owner, Repo: repo, Branch: branch, MaxFiles: maxFiles})
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TaskReviewRepo, payload)
	info, err := c.client.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(30*time.Minute))
	if err != nil {
		return "", err
	}
	c.log.Info("repo review enqueued", zap.String("task_id", info.ID))
	return info.ID, nil
}

func (c *Client) GetJobStatus(taskID string) (*JobStatus, error) {
	if !c.enabled {
		return nil, fmt.Errorf("queue is not enabled")
	}

	info, err := c.inspector.GetTaskInfo("default", taskID)
	if err != nil {
		return nil, err
	}

	status := &JobStatus{
		ID:      info.ID,
		Type:    info.Type,
		State:   info.State.String(),
		Created: info.NextProcessAt,
	}

	if info.State == asynq.TaskStateCompleted && len(info.Result) > 0 {
		status.Result = info.Result
	}
	if info.LastErr != "" {
		status.Error = info.LastErr
	}
	return status, nil
}

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	log    *zap.Logger
}

type TaskHandler struct {
	OnPRReview   func(ctx context.Context, p PRReviewPayload) ([]byte, error)
	OnRepoReview func(ctx context.Context, p RepoReviewPayload) ([]byte, error)
	Log          *zap.Logger
}

func NewWorker(redisAddr string, handler TaskHandler, log *zap.Logger) *Worker {
	opt := asynq.RedisClientOpt{Addr: redisAddr}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: 3,
		Queues:      map[string]int{"default": 10},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskReviewPR, func(ctx context.Context, t *asynq.Task) error {
		var p PRReviewPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		result, err := handler.OnPRReview(ctx, p)
		if err != nil {
			return err
		}
		_, writeErr := t.ResultWriter().Write(result)
		return writeErr
	})

	mux.HandleFunc(TaskReviewRepo, func(ctx context.Context, t *asynq.Task) error {
		var p RepoReviewPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		result, err := handler.OnRepoReview(ctx, p)
		if err != nil {
			return err
		}
		_, writeErr := t.ResultWriter().Write(result)
		return writeErr
	})

	return &Worker{server: srv, mux: mux, log: log.Named("worker")}
}

func (w *Worker) Run() error {
	w.log.Info("async worker started")
	return w.server.Run(w.mux)
}

func (w *Worker) Shutdown() {
	w.server.Shutdown()
}
