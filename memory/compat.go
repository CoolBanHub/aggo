package memory

// Backward compatibility: re-export builtin types and functions.
// Deprecated: import github.com/CoolBanHub/aggo/memory/builtin directly.
import "github.com/CoolBanHub/aggo/memory/builtin"

type (
	MemoryConfig            = builtin.MemoryConfig
	MemoryStorage           = builtin.MemoryStorage
	UserMemory              = builtin.UserMemory
	SessionSummary          = builtin.SessionSummary
	ConversationMessage     = builtin.ConversationMessage
	MemoryRetrieval         = builtin.MemoryRetrieval
	SummaryTriggerConfig    = builtin.SummaryTriggerConfig
	SummaryTriggerStrategy  = builtin.SummaryTriggerStrategy
	CleanupConfig           = builtin.CleanupConfig
	TaskQueueStats          = builtin.TaskQueueStats
	UserMemoryAnalyzerParam = builtin.UserMemoryAnalyzerParam
)

const (
	RetrievalLastN    = builtin.RetrievalLastN
	RetrievalFirstN   = builtin.RetrievalFirstN
	RetrievalSemantic = builtin.RetrievalSemantic

	UserMemoryOpUpdate = builtin.UserMemoryOpUpdate
	UserMemoryOpNoop   = builtin.UserMemoryOpNoop

	TriggerAlways     = builtin.TriggerAlways
	TriggerByMessages = builtin.TriggerByMessages
	TriggerByTime     = builtin.TriggerByTime
	TriggerSmart      = builtin.TriggerSmart
)

var (
	NewMemoryManager    = builtin.NewMemoryManager
	DefaultMemoryConfig = builtin.DefaultMemoryConfig
)
