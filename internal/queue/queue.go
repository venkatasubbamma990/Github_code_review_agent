package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

const (
	TaskReviewPR   = "review:pr"
	TaskReviewRepo = "review:repo"

	defaultQueue     = "default"
	defaultMaxRetry  = 3
	defaultTimeout   = 30 * time.Minute
	defaultRetention = 24 * time.Hour
)

type PRReviewPayload struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
	HeadSHA  string `json:"head_sha,omitempty"`
}

// EnqueueResult is returned after attempting to enqueue a review job.
type EnqueueResult struct {
	JobID        string
	Deduplicated bool
}

type RepoReviewPayload struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	MaxFiles int    `json:"max_files"`
}

// JobStatus is the inspectable status of an async review job.
type JobStatus struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	State         string          `json:"state"`
	Queue         string          `json:"queue,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         string          `json:"error,omitempty"`
	Retried       int             `json:"retried"`
	MaxRetry      int             `json:"max_retry"`
	NextProcessAt *time.Time      `json:"next_process_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	LastFailedAt  *time.Time      `json:"last_failed_at,omitempty"`
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

func enqueueOptions(extra ...asynq.Option) []asynq.Option {
	opts := []asynq.Option{
		asynq.Queue(defaultQueue),
		asynq.MaxRetry(defaultMaxRetry),
		asynq.Timeout(defaultTimeout),
		// Keep completed tasks (and their ResultWriter payload) queryable.
		asynq.Retention(defaultRetention),
	}
	return append(opts, extra...)
}

// PRReviewTaskID builds a stable idempotency key for a PR head SHA.
// Duplicate webhook deliveries for the same commit reuse this task ID.
func PRReviewTaskID(owner, repo string, prNumber int, headSHA string) string {
	sha := strings.TrimSpace(headSHA)
	if sha == "" {
		sha = "unknown"
	}
	return fmt.Sprintf("review:pr:%s/%s#%d@%s", owner, repo, prNumber, sha)
}

func (c *Client) EnqueuePRReview(owner, repo string, prNumber int, headSHA string) (*EnqueueResult, error) {
	payload, err := json.Marshal(PRReviewPayload{
		Owner:    owner,
		Repo:     repo,
		PRNumber: prNumber,
		HeadSHA:  headSHA,
	})
	if err != nil {
		return nil, err
	}

	taskID := PRReviewTaskID(owner, repo, prNumber, headSHA)
	task := asynq.NewTask(TaskReviewPR, payload)
	info, err := c.client.Enqueue(task, enqueueOptions(asynq.TaskID(taskID))...)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			c.log.Info("PR review already queued (idempotent)",
				zap.String("task_id", taskID),
				zap.String("repo", owner+"/"+repo),
				zap.Int("pr", prNumber),
				zap.String("sha", headSHA),
			)
			return &EnqueueResult{JobID: taskID, Deduplicated: true}, nil
		}
		return nil, err
	}

	c.log.Info("PR review enqueued",
		zap.String("task_id", info.ID),
		zap.Duration("retention", defaultRetention),
	)
	return &EnqueueResult{JobID: info.ID, Deduplicated: false}, nil
}

func (c *Client) EnqueueRepoReview(owner, repo, branch string, maxFiles int) (*EnqueueResult, error) {
	payload, err := json.Marshal(RepoReviewPayload{Owner: owner, Repo: repo, Branch: branch, MaxFiles: maxFiles})
	if err != nil {
		return nil, err
	}
	task := asynq.NewTask(TaskReviewRepo, payload)
	info, err := c.client.Enqueue(task, enqueueOptions()...)
	if err != nil {
		return nil, err
	}
	c.log.Info("repo review enqueued",
		zap.String("task_id", info.ID),
		zap.Duration("retention", defaultRetention),
	)
	return &EnqueueResult{JobID: info.ID, Deduplicated: false}, nil
}

func (c *Client) GetJobStatus(taskID string) (*JobStatus, error) {
	if !c.enabled {
		return nil, fmt.Errorf("queue is not enabled")
	}
	if taskID == "" {
		return nil, fmt.Errorf("task id is required")
	}

	info, err := c.inspector.GetTaskInfo(defaultQueue, taskID)
	if err != nil {
		return nil, err
	}
	return taskInfoToJobStatus(info), nil
}

func taskInfoToJobStatus(info *asynq.TaskInfo) *JobStatus {
	status := &JobStatus{
		ID:       info.ID,
		Type:     info.Type,
		State:    normalizeJobState(info.State),
		Queue:    info.Queue,
		Retried:  info.Retried,
		MaxRetry: info.MaxRetry,
	}

	if len(info.Result) > 0 {
		status.Result = json.RawMessage(info.Result)
	}
	if info.LastErr != "" {
		status.Error = info.LastErr
	}
	if !info.NextProcessAt.IsZero() {
		t := info.NextProcessAt.UTC()
		status.NextProcessAt = &t
	}
	if !info.CompletedAt.IsZero() {
		t := info.CompletedAt.UTC()
		status.CompletedAt = &t
	}
	if !info.LastFailedAt.IsZero() {
		t := info.LastFailedAt.UTC()
		status.LastFailedAt = &t
	}
	return status
}

// normalizeJobState maps asynq states to API-friendly values.
// Archived tasks (exhausted retries) are exposed as "failed".
func normalizeJobState(state asynq.TaskState) string {
	switch state {
	case asynq.TaskStatePending:
		return "pending"
	case asynq.TaskStateActive:
		return "active"
	case asynq.TaskStateScheduled:
		return "scheduled"
	case asynq.TaskStateRetry:
		return "retry"
	case asynq.TaskStateCompleted:
		return "completed"
	case asynq.TaskStateArchived:
		return "failed"
	case asynq.TaskStateAggregating:
		return "aggregating"
	default:
		return state.String()
	}
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
		Queues:      map[string]int{defaultQueue: 10},
	})

	workerLog := log.Named("worker")
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskReviewPR, func(ctx context.Context, t *asynq.Task) error {
		return handleTask(ctx, t, workerLog, func(ctx context.Context) ([]byte, error) {
			var p PRReviewPayload
			if err := json.Unmarshal(t.Payload(), &p); err != nil {
				return nil, fmt.Errorf("decode PR payload: %w", err)
			}
			return handler.OnPRReview(ctx, p)
		})
	})

	mux.HandleFunc(TaskReviewRepo, func(ctx context.Context, t *asynq.Task) error {
		return handleTask(ctx, t, workerLog, func(ctx context.Context) ([]byte, error) {
			var p RepoReviewPayload
			if err := json.Unmarshal(t.Payload(), &p); err != nil {
				return nil, fmt.Errorf("decode repo payload: %w", err)
			}
			return handler.OnRepoReview(ctx, p)
		})
	})

	return &Worker{server: srv, mux: mux, log: workerLog}
}

func handleTask(
	ctx context.Context,
	t *asynq.Task,
	log *zap.Logger,
	run func(context.Context) ([]byte, error),
) error {
	result, err := run(ctx)
	if err != nil {
		log.Error("task failed", zap.String("type", t.Type()), zap.Error(err))
		return err
	}

	if len(result) == 0 {
		result = []byte("{}")
	}
	if _, writeErr := t.ResultWriter().Write(result); writeErr != nil {
		log.Error("failed to persist task result",
			zap.String("type", t.Type()),
			zap.Error(writeErr),
		)
		return fmt.Errorf("write task result: %w", writeErr)
	}

	log.Info("task completed",
		zap.String("type", t.Type()),
		zap.Int("result_bytes", len(result)),
	)
	return nil
}

func (w *Worker) Run() error {
	w.log.Info("async worker started")
	return w.server.Run(w.mux)
}

func (w *Worker) Shutdown() {
	w.server.Shutdown()
}
