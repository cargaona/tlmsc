package download

import (
	"sync"
	"tlmsc/internal/streamrip"
)

type DownloadCallback func(job *streamrip.DownloadJob, progress streamrip.Progress)

type Queue struct {
	jobs   chan *streamrip.DownloadJob
	active *streamrip.DownloadJob
	mu     sync.Mutex
}

func NewQueue(bufferSize int) *Queue {
	return &Queue{
		jobs: make(chan *streamrip.DownloadJob, bufferSize),
	}
}

func (q *Queue) Enqueue(job *streamrip.DownloadJob) {
	q.jobs <- job
}

func (q *Queue) GetActive() *streamrip.DownloadJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active
}

func (q *Queue) setActive(job *streamrip.DownloadJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.active = job
}

func (q *Queue) Close() {
	close(q.jobs)
}

func (q *Queue) GetChannel() chan *streamrip.DownloadJob {
	return q.jobs
}
