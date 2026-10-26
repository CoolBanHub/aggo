package memory

import (
	"context"
	"errors"
	"log"
)

var (
	ErrMemoryMiddlewareClosed = errors.New("memory: middleware is closed")
	ErrMemorizeQueueFull      = errors.New("memory: memorize queue is full")
)

type memorizeTask struct {
	ctx context.Context
	req *MemorizeRequest
}

func (m *MemoryMiddleware) isClosed() bool {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return m.closed
}

func (m *MemoryMiddleware) enqueueMemorize(ctx context.Context, req *MemorizeRequest) error {
	m.writeMu.Lock()
	if m.closed {
		m.writeMu.Unlock()
		return ErrMemoryMiddlewareClosed
	}
	if async, ok := m.provider.(AsyncMemorizer); ok {
		m.pendingWrites.Add(1)
		m.writeMu.Unlock()
		err := async.EnqueueMemorize(ctx, req)
		m.pendingWrites.Done()
		return err
	}
	select {
	case m.writeQueue <- memorizeTask{ctx: context.WithoutCancel(ctx), req: req}:
		m.pendingWrites.Add(1)
		if !m.writerStarted {
			m.writerStarted = true
			go m.runMemorizeWorker()
		}
		m.writeMu.Unlock()
		return nil
	default:
		m.writeMu.Unlock()
		return ErrMemorizeQueueFull
	}
}

func (m *MemoryMiddleware) runMemorizeWorker() {
	for {
		// Exit when idle so existing callers that only close the provider do not
		// retain an idle worker. enqueueMemorize restarts it under the same lock.
		m.writeMu.Lock()
		var task memorizeTask
		select {
		case next, ok := <-m.writeQueue:
			if !ok {
				m.writerStarted = false
				close(m.writerDone)
				m.writeMu.Unlock()
				return
			}
			task = next
		default:
			m.writerStarted = false
			m.writeMu.Unlock()
			return
		}
		m.writeMu.Unlock()
		ctx, cancel := context.WithTimeout(task.ctx, m.config.MemorizeTimeout)
		err := m.provider.Memorize(ctx, task.req)
		cancel()
		if err != nil {
			log.Printf("MemoryMiddleware: Memorize failed: %v", err)
			m.writeMu.Lock()
			if m.writeErr == nil {
				m.writeErr = err
			}
			m.writeMu.Unlock()
		}
		m.pendingWrites.Done()
	}
}

// Shutdown stops new writes and drains accepted fallback writes.
// Call it after active runs have finished. Timeout only stops waiting; draining
// continues in the background and a later Shutdown or Close can wait again.
// It is safe to call concurrently. Call the provider's Close after Shutdown;
// providers with AsyncMemorizer must drain their own accepted writes in Close.
// The first fallback write error is returned.
func (m *MemoryMiddleware) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m.writeMu.Lock()
	if !m.closed {
		m.closed = true
		if m.writeQueue != nil {
			close(m.writeQueue)
		}
		if !m.writerStarted {
			close(m.writerDone)
		}
		go m.finishShutdown()
	}
	m.writeMu.Unlock()

	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	default:
	}
	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *MemoryMiddleware) finishShutdown() {
	m.pendingWrites.Wait()
	<-m.writerDone
	m.writeMu.Lock()
	m.shutdownErr = m.writeErr
	m.writeMu.Unlock()
	close(m.shutdownDone)
}
