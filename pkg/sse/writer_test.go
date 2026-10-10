package sse

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// recordResponseWriter 记录每次 Write 调用的字节，用于检测帧交错。
type recordResponseWriter struct {
	mu     sync.Mutex
	chunks []string
	fail   bool
}

func (w *recordResponseWriter) Header() http.Header { return http.Header{} }

func (w *recordResponseWriter) WriteHeader(status int) {}

func (w *recordResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fail {
		return 0, errors.New("connection reset")
	}
	w.chunks = append(w.chunks, string(p))
	return len(p), nil
}

func (w *recordResponseWriter) Flush() {}

func (w *recordResponseWriter) recorded() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.chunks...)
}

// 并发写事件帧不会交错：每个 Write 调用都是一个完整的帧。
func TestConcurrentWritesDoNotInterleave(t *testing.T) {
	rw := &recordResponseWriter{}
	w := NewWriter("w1", rw)
	if w == nil {
		t.Fatal("NewWriter returned nil")
	}

	const workers = 8
	const perWorker = 50
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if err := w.WriteEventString("", "tick", "payload"); err != nil {
					t.Errorf("WriteEventString: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	chunks := rw.recorded()
	if len(chunks) != workers*perWorker {
		t.Fatalf("recorded %d chunks, want %d", len(chunks), workers*perWorker)
	}
	for _, chunk := range chunks {
		if !strings.HasPrefix(chunk, "event: tick\n") || !strings.HasSuffix(chunk, "data: payload\n\n") {
			t.Fatalf("interleaved or malformed frame: %q", chunk)
		}
	}
}

// Close 幂等；关闭后所有写方法返回 ErrWriterClosed。
func TestCloseIsIdempotentAndRejectsWrites(t *testing.T) {
	rw := &recordResponseWriter{}
	w := NewWriter("w1", rw)

	if err := w.WriteDataString("before"); err != nil {
		t.Fatalf("write before close: %v", err)
	}

	w.Close()
	w.Close() // 幂等

	if !w.IsClosed() {
		t.Fatal("writer should be closed")
	}
	for name, err := range map[string]error{
		"data":    w.WriteDataString("after"),
		"comment": w.WriteComment("after"),
		"done":    w.WriteDone(),
		"event":   w.WriteEventString("", "e", "after"),
	} {
		if !errors.Is(err, ErrWriterClosed) {
			t.Fatalf("%s after close = %v, want ErrWriterClosed", name, err)
		}
	}

	chunks := rw.recorded()
	if len(chunks) != 1 {
		t.Fatalf("writes after close must not reach the connection: %v", chunks)
	}
}

// 第一次写失败会标记关闭并保留原始错误，后续写入返回同一错误。
func TestFirstWriteErrorIsPreserved(t *testing.T) {
	rw := &recordResponseWriter{fail: true}
	w := NewWriter("w1", rw)

	first := w.WriteDataString("boom")
	if first == nil {
		t.Fatal("expected write error")
	}
	if !w.IsClosed() {
		t.Fatal("writer should be closed after first write failure")
	}

	second := w.WriteComment("again")
	if !errors.Is(second, first) {
		t.Fatalf("subsequent write = %v, want original error %v", second, first)
	}
}
