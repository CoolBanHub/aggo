package memory

import (
	"context"
	"fmt"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"sync/atomic"
	"testing"
	"time"
)

type countingAnalysisModel struct{ calls atomic.Int64 }

func (m *countingAnalysisModel) Generate(context.Context, []*schema.AgenticMessage, ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.calls.Add(1)
	return agmsg.AssistantMessage(`{"op":"noop"}`), nil
}
func (m *countingAnalysisModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg, err := m.Generate(ctx, input, opts...)
	reader, writer := schema.Pipe[*schema.AgenticMessage](1)
	writer.Send(msg, err)
	writer.Close()
	return reader, nil
}

func TestEnqueueMemorizeLifecycleIncludesAnalysis(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprintf("shutdown=%v", shutdown), func(t *testing.T) {
			cfg := builtin.DefaultMemoryConfig()
			cfg.EnableUserMemories = true
			cfg.EnableSessionSummary = false
			cfg.AsyncWorkerPoolSize = 1
			cfg.DebounceWindowSeconds = intPtr(60)
			model := &countingAnalysisModel{}
			store := storage.NewMemoryStore()
			manager, err := builtin.NewMemoryManager(model, store, cfg)
			if err != nil {
				t.Fatal(err)
			}
			provider := &builtinProvider{MemoryManager: manager}
			defer provider.Close()
			entered, release := make(chan struct{}), make(chan struct{})
			if err := manager.EnqueueTask("block", "u", "s", func(context.Context) { close(entered); <-release }); err != nil {
				t.Fatal(err)
			}
			<-entered
			if err := provider.EnqueueMemorize(context.Background(), &MemorizeRequest{UserID: "u", SessionID: "s", Messages: []*schema.AgenticMessage{schema.UserAgenticMessage("question"), agmsg.AssistantMessage("answer")}}); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if shutdown {
					done <- manager.Shutdown(ctx)
				} else {
					done <- manager.Flush(ctx)
				}
			}()
			if shutdown {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				_ = manager.Shutdown(ctx)
				cancel()
			} else {
				select {
				case err := <-done:
					t.Fatalf("Flush returned early: %v", err)
				case <-time.After(20 * time.Millisecond):
				}
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if model.calls.Load() != 1 {
				t.Fatalf("analysis calls after return = %d", model.calls.Load())
			}
			messages, err := store.GetMessages(context.Background(), "s", "u", 0)
			if err != nil || len(messages) != 2 {
				t.Fatalf("messages=%d err=%v", len(messages), err)
			}
		})
	}
}
