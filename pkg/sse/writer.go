package sse

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
)

type Writer struct {
	id      string
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
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
		closed:  false,
	}
}

func (w *Writer) Close() {
	w.closed = true
}

func (w *Writer) IsClosed() bool {
	return w.closed
}

func (w *Writer) WriteEvent(event *Event) error {
	if event == nil || w.closed {
		return nil
	}

	_, err := w.w.Write([]byte(event.String()))
	if err != nil {
		w.closed = true
		return err
	}

	w.flusher.Flush()
	return nil
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
	_, err := fmt.Fprintf(w.w, ": %s\n\n", comment)
	if err != nil {
		return err
	}

	w.flusher.Flush()
	return nil
}

func (w *Writer) WriteKeepAlive() error {
	return w.WriteComment("keep-alive")
}

func (w *Writer) WriteDone() error {
	_, err := fmt.Fprintf(w.w, "data: [DONE]\n\n")
	if err != nil {
		return err
	}

	w.flusher.Flush()
	return nil
}

func (w *Writer) Stream(ctx context.Context, stream *schema.StreamReader[*schema.Message], fn func(output *schema.Message) any) error {
	if fn == nil {
		fn = func(output *schema.Message) any {
			return output
		}
	}

	for {
		// 检查上下文是否被取消或 Writer 是否已关闭
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if w.IsClosed() {
				return fmt.Errorf("writer is closed")
			}
		}

		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil // 正常结束
			}
			return err
		}

		// 如果 chunk 为空为空，则结束流
		if chunk == nil || chunk.Content == "" {
			continue
		}
		newChunk := fn(chunk)
		if newChunk == nil {
			continue
		}
		b, err := sonic.Marshal(newChunk)
		if err != nil {
			return err
		}

		if err = w.WriteData(b); err != nil {
			return err
		}
	}
	return w.WriteDone()
}
