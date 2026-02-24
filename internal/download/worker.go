package download

import (
	"context"
	"fmt"
	"tlmsc/internal/streamrip"
)

type Worker struct {
	queue    *Queue
	client   *streamrip.Client
	callback DownloadCallback
	debug    bool
}

func NewWorker(queue *Queue, client *streamrip.Client, callback DownloadCallback, debug bool) *Worker {
	return &Worker{
		queue:    queue,
		client:   client,
		callback: callback,
		debug:    debug,
	}
}

// ProcessQueue starts the worker loop that processes download jobs
func (w *Worker) ProcessQueue(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case job := <-w.queue.GetChannel():
			if job == nil {
				return nil
			}

			w.queue.setActive(job)
			w.executeDownload(job)
			w.queue.setActive(nil)
		}
	}
}

// executeDownload executes a single download with retry logic
func (w *Worker) executeDownload(job *streamrip.DownloadJob) {
	progressCh := make(chan streamrip.Progress, 10)
	defer close(progressCh)

	// Try primary source first
	err := w.download(job, progressCh)
	if err == nil {
		return
	}

	if w.debug {
		fmt.Printf("[worker] Primary source %s failed: %v\n", job.Source, err)
	}

	// Retry with fallback source
	if job.Retries < 1 {
		job.Retries++

		// Swap to fallback source
		if job.Source == "qobuz" {
			job.Source = "deezer"
		} else {
			job.Source = "qobuz"
		}

		if w.debug {
			fmt.Printf("[worker] Retrying with fallback source: %s\n", job.Source)
		}

		w.executeDownload(job)
	} else {
		// Both sources failed
		w.callback(job, streamrip.Progress{
			Status:  "failed",
			Percent: 0,
		})
	}
}

// download performs the actual download for a job
func (w *Worker) download(job *streamrip.DownloadJob, progressCh chan streamrip.Progress) error {
	// Notify start
	w.callback(job, streamrip.Progress{
		Status:  "starting",
		Percent: 0,
	})

	// Start progress handler goroutine
	go func() {
		for progress := range progressCh {
			w.callback(job, progress)
		}
	}()

	// Execute download: rip id <source> album <albumID>
	err := w.client.Download(job.Album.ID, job.Source, progressCh)
	return err
}
