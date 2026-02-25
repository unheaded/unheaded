package logger

import (
	"sync"
	"sync/atomic"

	"unheaded/pkg/audit"
)

// AsyncProcessor handles asynchronous event processing with worker pool.
type AsyncProcessor struct {
	workers    int
	handler    func(*audit.AuditEvent) error
	queue      chan *audit.AuditEvent
	wg         sync.WaitGroup
	stopCh     chan struct{}
	stopped    int32
	errorCount int64
	processed  int64
}

// NewAsyncProcessor creates a new AsyncProcessor.
func NewAsyncProcessor(workers int, handler func(*audit.AuditEvent) error) *AsyncProcessor {
	if workers <= 0 {
		workers = 1
	}
	return &AsyncProcessor{
		workers: workers,
		handler: handler,
		queue:   make(chan *audit.AuditEvent, workers*100),
		stopCh:  make(chan struct{}),
	}
}

// Start begins the worker pool.
func (p *AsyncProcessor) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop gracefully shuts down the processor.
func (p *AsyncProcessor) Stop() {
	if !atomic.CompareAndSwapInt32(&p.stopped, 0, 1) {
		return
	}
	close(p.stopCh)
	p.wg.Wait()
}

// Submit adds an event to the processing queue.
func (p *AsyncProcessor) Submit(event *audit.AuditEvent) bool {
	if atomic.LoadInt32(&p.stopped) == 1 {
		return false
	}

	select {
	case p.queue <- event:
		return true
	default:
		// Queue is full, process synchronously to avoid losing events
		if err := p.handler(event); err != nil {
			atomic.AddInt64(&p.errorCount, 1)
		}
		atomic.AddInt64(&p.processed, 1)
		return true
	}
}

// worker is the main worker loop.
func (p *AsyncProcessor) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			// Drain remaining events before stopping
			p.drainQueue()
			return
		case event, ok := <-p.queue:
			if !ok {
				return
			}
			if err := p.handler(event); err != nil {
				atomic.AddInt64(&p.errorCount, 1)
			}
			atomic.AddInt64(&p.processed, 1)
		}
	}
}

// drainQueue processes remaining events in the queue.
func (p *AsyncProcessor) drainQueue() {
	for {
		select {
		case event, ok := <-p.queue:
			if !ok {
				return
			}
			if err := p.handler(event); err != nil {
				atomic.AddInt64(&p.errorCount, 1)
			}
			atomic.AddInt64(&p.processed, 1)
		default:
			return
		}
	}
}

// QueueSize returns the current queue size.
func (p *AsyncProcessor) QueueSize() int {
	return len(p.queue)
}

// ProcessedCount returns the number of processed events.
func (p *AsyncProcessor) ProcessedCount() int64 {
	return atomic.LoadInt64(&p.processed)
}

// ErrorCount returns the number of errors encountered.
func (p *AsyncProcessor) ErrorCount() int64 {
	return atomic.LoadInt64(&p.errorCount)
}

// AsyncStats contains async processor statistics.
type AsyncStats struct {
	QueueSize      int   `json:"queue_size"`
	QueueCapacity  int   `json:"queue_capacity"`
	ProcessedCount int64 `json:"processed_count"`
	ErrorCount     int64 `json:"error_count"`
	Workers        int   `json:"workers"`
}

// Stats returns current processor statistics.
func (p *AsyncProcessor) Stats() *AsyncStats {
	return &AsyncStats{
		QueueSize:      len(p.queue),
		QueueCapacity:  cap(p.queue),
		ProcessedCount: atomic.LoadInt64(&p.processed),
		ErrorCount:     atomic.LoadInt64(&p.errorCount),
		Workers:        p.workers,
	}
}
