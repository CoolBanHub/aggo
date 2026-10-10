package sse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
)

// ErrWriterClosed 在 Writer 关闭后继续写入时返回。
// 旧版本对已关闭的写入静默返回 nil，调用方无法感知连接已结束。
var ErrWriterClosed = errors.New("sse: writer is closed")

type Writer struct {
	id      string
	w       http.ResponseWriter
	flusher http.Flusher

	// mu 保护 closed/err 以及底层 ResponseWriter 的写入，
	// 保证并发写事件帧不会交错；第一次写失败会记录 err 并标记关闭
	mu     sync.Mutex
	closed bool
	err    error

	Done func(http.ResponseWriter) error
}

func NewWriter(id string, w http.ResponseWriter) *Writer {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	return &Writer{
		id:      id,
		w:       w,
		flusher: flusher,
	}
}

func (w *Writer) SetDone(f func(http.ResponseWriter) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Done = f
}

// Close 幂等；关闭后所有写方法返回 ErrWriterClosed
func (w *Writer) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

func (w *Writer) IsClosed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// writeFrame 串行化“写入 + Flush”，保证事件帧不交错。
// 第一次写失败会保留原始错误并标记关闭，之后的写入直接返回该错误。
func (w *Writer) writeFrame(frame string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.err != nil {
		return w.err
	}
	if w.closed {
		return ErrWriterClosed
	}

	if _, err := io.WriteString(w.w, frame); err != nil {
		w.err = err
		w.closed = true
		return err
	}
	w.flusher.Flush()
	return nil
}

func (w *Writer) WriteEvent(event *Event) error {
	if event == nil {
		return nil
	}
	return w.writeFrame(event.String())
}

func (w *Writer) WriteEventSimple(id, eventType string, data []byte) error {
	event := NewEvent()
	defer event.Release()

	if id != "" {
		event.SetID(id)
	}
	if eventType != "" {
		event.SetEvent(eventType)
	}
	if data != nil {
		event.SetData(data)
	}

	return w.WriteEvent(event)
}

func (w *Writer) WriteEventString(id, eventType, data string) error {
	return w.WriteEventSimple(id, eventType, []byte(data))
}

func (w *Writer) WriteEventJSON(id, eventType string, data interface{}) error {
	jsonData, err := sonic.Marshal(data)
	if err != nil {
		return err
	}

	return w.WriteEventSimple(id, eventType, jsonData)
}

func (w *Writer) WriteJSONData(data interface{}) error {
	jsonData, err := sonic.Marshal(data)
	if err != nil {
		return err
	}

	return w.WriteData(jsonData)
}

func (w *Writer) WriteData(data []byte) error {
	event := NewEvent()
	defer event.Release()
	event.SetID(w.id)
	event.SetData(data)
	return w.WriteEvent(event)
}

func (w *Writer) WriteDataString(data string) error {
	return w.WriteData([]byte(data))
}

func (w *Writer) WriteComment(comment string) error {
	return w.writeFrame(fmt.Sprintf(": %s\n\n", comment))
}

func (w *Writer) WriteKeepAlive() error {
	return w.WriteComment("keep-alive")
}

func (w *Writer) WriteDone() error {
	w.mu.Lock()
	done := w.Done
	w.mu.Unlock()

	if done != nil {
		if err := done(w.w); err != nil {
			return w.markWriteFailed(err)
		}
		return w.writeFrame("")
	}

	return w.writeFrame("data: [DONE]\n\n")
}

// markWriteFailed 在 Done 回调自行写入并失败时，与 writeFrame 保持
// 相同的“记录首次错误并标记关闭”语义。
func (w *Writer) markWriteFailed(err error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err == nil {
		w.err = err
		w.closed = true
	}
	return w.err
}

func (w *Writer) Stream(ctx context.Context, stream *schema.StreamReader[*schema.AgenticMessage], fn func(output *schema.AgenticMessage, index int) any) error {
	if fn == nil {
		fn = func(output *schema.AgenticMessage, index int) any {
			return output
		}
	}

	index := 0
	for {
		// 检查上下文是否被取消或 Writer 是否已关闭
		if err := ctx.Err(); err != nil {
			return err
		}
		if w.IsClosed() {
			return ErrWriterClosed
		}

		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return w.WriteDone() // 正常结束
			}
			return err
		}

		// 如果 chunk 为空为空，则结束流
		if chunk == nil || (agmsg.Text(chunk) == "" && !agmsg.HasFunctionToolCall(chunk)) {
			continue
		}
		newChunk := fn(chunk, index)
		if newChunk == nil {
			continue
		}
		index++
		b, err := sonic.Marshal(newChunk)
		if err != nil {
			return err
		}

		if err = w.WriteData(b); err != nil {
			return err
		}
	}
}
