package collection

import (
	"errors"
	"testing"
	"time"
)

func validTask() *Task {
	return &Task{ID: "task-1", TenantID: 1, UserID: 2, KnowledgeBaseID: "kb-1", IdempotencyKey: "create-1", Status: TaskStatusDraft, Limits: Limits{MaxSources: 2, MaxResponseBytes: 1024}}
}

func TestTaskValidate(t *testing.T) {
	task := validTask()
	if err := task.Validate(); err != nil {
		t.Fatalf("valid task: %v", err)
	}
	task.Limits.MaxResponseBytes = 0
	if !errors.Is(task.Validate(), ErrInvalidTask) {
		t.Fatal("expected invalid task for unbounded response")
	}
}

func TestTaskLifecycle(t *testing.T) {
	task := validTask()
	if err := task.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := task.Complete(); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !task.Status.IsTerminal() {
		t.Fatal("completed task must be terminal")
	}
	if !errors.Is(task.Cancel(), ErrInvalidTransition) {
		t.Fatal("terminal task must not be cancelled")
	}
}

func TestEnabledScheduleRequiresIntervalAndDueTime(t *testing.T) {
	task := validTask()
	task.Schedule.Enabled = true
	if !errors.Is(task.Validate(), ErrInvalidTask) {
		t.Fatal("enabled schedule without interval and due time must be invalid")
	}
	now := time.Now().UTC()
	task.Schedule.IntervalSeconds, task.Schedule.NextRunAt = 60, &now
	if err := task.Validate(); err != nil {
		t.Fatalf("valid schedule: %v", err)
	}
}

func TestSourceRequiresHTTPS(t *testing.T) {
	valid := &Source{ID: "source-1", TaskID: "task-1", URL: "https://example.com/docs"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid source: %v", err)
	}
	valid.URL = "http://example.com"
	if !errors.Is(valid.Validate(), ErrInvalidSource) {
		t.Fatal("expected HTTP source rejection")
	}
}
