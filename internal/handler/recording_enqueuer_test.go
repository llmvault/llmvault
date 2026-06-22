package handler_test

import (
	"context"

	"github.com/hibiken/asynq"
)

type recordingEnqueuer struct {
	tasks []*asynq.Task
}

func (r *recordingEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return r.EnqueueContext(context.Background(), task, opts...)
}

func (r *recordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (r *recordingEnqueuer) Close() error { return nil }
