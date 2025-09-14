package queue

import (
	"context"
)

// Variant — выбранный пользователем вариант загрузки

type Variant string

const (
	VarVideo360 Variant = "video360"
	VarVideo720 Variant = "video720"
	VarAudioMP3 Variant = "audioMp3"
)

// Job — задача на загрузку

type Job struct {
	ChatID      int64
	URL         string
	Variant     Variant
	RequestedAt int64
	Attempts    int
}

// Queue — простая очередь с воркерами

type Queue struct {
	ch      chan Job
	workers int
}

func NewQueue(capacity, workers int) *Queue {
	if capacity <= 0 { capacity = 100 }
	if workers <= 0 { workers = 2 }
	return &Queue{ch: make(chan Job, capacity), workers: workers}
}

func (q *Queue) Enqueue(j Job) { q.ch <- j }

func (q *Queue) Start(ctx context.Context, worker func(ctx context.Context, job Job)) {
	for i := 0; i < q.workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-q.ch:
					worker(ctx, j)
				}
			}
		}()
	}
}
