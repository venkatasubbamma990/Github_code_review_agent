package queue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestNormalizeJobState(t *testing.T) {
	cases := []struct {
		in   asynq.TaskState
		want string
	}{
		{asynq.TaskStatePending, "pending"},
		{asynq.TaskStateActive, "active"},
		{asynq.TaskStateScheduled, "scheduled"},
		{asynq.TaskStateRetry, "retry"},
		{asynq.TaskStateCompleted, "completed"},
		{asynq.TaskStateArchived, "failed"},
		{asynq.TaskStateAggregating, "aggregating"},
	}
	for _, tc := range cases {
		if got := normalizeJobState(tc.in); got != tc.want {
			t.Errorf("normalizeJobState(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTaskInfoToJobStatus_CompletedWithResult(t *testing.T) {
	completed := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	info := &asynq.TaskInfo{
		ID:          "job-1",
		Type:        TaskReviewRepo,
		Queue:       defaultQueue,
		State:       asynq.TaskStateCompleted,
		MaxRetry:    3,
		Retried:     0,
		Result:      []byte(`{"quality":{"overall":80}}`),
		CompletedAt: completed,
		Retention:   defaultRetention,
	}

	status := taskInfoToJobStatus(info)
	if status.State != "completed" {
		t.Fatalf("state = %q, want completed", status.State)
	}
	if status.ID != "job-1" || status.Type != TaskReviewRepo {
		t.Fatalf("unexpected id/type: %+v", status)
	}
	if status.CompletedAt == nil || !status.CompletedAt.Equal(completed) {
		t.Fatalf("completed_at = %v, want %v", status.CompletedAt, completed)
	}
	if !json.Valid(status.Result) {
		t.Fatalf("result is not valid JSON: %s", status.Result)
	}
	if string(status.Result) != `{"quality":{"overall":80}}` {
		t.Fatalf("result = %s", status.Result)
	}
}

func TestTaskInfoToJobStatus_FailedArchived(t *testing.T) {
	failedAt := time.Date(2026, 7, 23, 11, 0, 0, 0, time.UTC)
	info := &asynq.TaskInfo{
		ID:           "job-2",
		Type:         TaskReviewPR,
		Queue:        defaultQueue,
		State:        asynq.TaskStateArchived,
		MaxRetry:     3,
		Retried:      3,
		LastErr:      "llm timeout",
		LastFailedAt: failedAt,
	}

	status := taskInfoToJobStatus(info)
	if status.State != "failed" {
		t.Fatalf("state = %q, want failed", status.State)
	}
	if status.Error != "llm timeout" {
		t.Fatalf("error = %q", status.Error)
	}
	if status.Retried != 3 || status.MaxRetry != 3 {
		t.Fatalf("retry fields: retried=%d max=%d", status.Retried, status.MaxRetry)
	}
	if status.LastFailedAt == nil || !status.LastFailedAt.Equal(failedAt) {
		t.Fatalf("last_failed_at = %v", status.LastFailedAt)
	}
	if status.Result != nil {
		t.Fatalf("expected nil result, got %s", status.Result)
	}
}

func TestTaskInfoToJobStatus_RetryPending(t *testing.T) {
	next := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	info := &asynq.TaskInfo{
		ID:            "job-3",
		Type:          TaskReviewPR,
		State:         asynq.TaskStateRetry,
		MaxRetry:      3,
		Retried:       1,
		LastErr:       "temporary error",
		NextProcessAt: next,
	}

	status := taskInfoToJobStatus(info)
	if status.State != "retry" {
		t.Fatalf("state = %q, want retry", status.State)
	}
	if status.NextProcessAt == nil || !status.NextProcessAt.Equal(next) {
		t.Fatalf("next_process_at = %v", status.NextProcessAt)
	}
}

func TestEnqueueOptionsIncludeRetention(t *testing.T) {
	opts := enqueueOptions()
	if len(opts) < 4 {
		t.Fatalf("expected enqueue options including retention, got %d", len(opts))
	}
	foundRetention := false
	for _, opt := range opts {
		if opt.Type() == asynq.RetentionOpt {
			foundRetention = true
			if opt.Value().(time.Duration) != defaultRetention {
				t.Fatalf("retention = %v, want %v", opt.Value(), defaultRetention)
			}
		}
	}
	if !foundRetention {
		t.Fatal("Retention option missing from enqueueOptions")
	}
}
