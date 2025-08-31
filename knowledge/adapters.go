package knowledge

import (
	"github.com/CoolBanHub/aggo/knowledge/chunking"
)

// ChunkingStrategyAdapter 将chunking包的实现适配为knowledge接口
type ChunkingStrategyAdapter struct {
	strategy interface {
		Chunk(doc chunking.Document) ([]chunking.Chunk, error)
		GetChunkSize() int
		GetChunkOverlap() int
		SetChunkSize(size int)
		SetChunkOverlap(overlap int)
	}
}

// NewFixedSizeChunkingStrategy 创建固定大小分块策略适配器
func NewFixedSizeChunkingStrategy(chunkSize, chunkOverlap int) ChunkingStrategy {
	return &ChunkingStrategyAdapter{
		strategy: chunking.NewFixedSizeChunkingStrategy(chunkSize, chunkOverlap),
	}
}

// NewSentenceChunkingStrategy 创建句子分块策略适配器
func NewSentenceChunkingStrategy(maxChunkSize, overlapSentences int) ChunkingStrategy {
	return &ChunkingStrategyAdapter{
		strategy: chunking.NewSentenceChunkingStrategy(maxChunkSize, overlapSentences),
	}
}

// Chunk 将文档分割成块
func (c *ChunkingStrategyAdapter) Chunk(doc Document) ([]Chunk, error) {
	// 转换为chunking包的Document类型
	chunkingDoc := chunking.Document{
		ID:        doc.ID,
		Content:   doc.Content,
		Metadata:  doc.Metadata,
		Vector:    doc.Vector,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}

	// 调用底层分块策略
	chunkingChunks, err := c.strategy.Chunk(chunkingDoc)
	if err != nil {
		return nil, err
	}

	// 转换回knowledge包的Chunk类型
	chunks := make([]Chunk, len(chunkingChunks))
	for i, chunk := range chunkingChunks {
		chunks[i] = Chunk{
			ID:          chunk.ID,
			DocumentID:  chunk.DocumentID,
			Content:     chunk.Content,
			Metadata:    chunk.Metadata,
			Vector:      chunk.Vector,
			Index:       chunk.Index,
			StartOffset: chunk.StartOffset,
			EndOffset:   chunk.EndOffset,
		}
	}

	return chunks, nil
}

// GetChunkSize 获取分块大小
func (c *ChunkingStrategyAdapter) GetChunkSize() int {
	return c.strategy.GetChunkSize()
}

// GetChunkOverlap 获取分块重叠大小
func (c *ChunkingStrategyAdapter) GetChunkOverlap() int {
	return c.strategy.GetChunkOverlap()
}

// SetChunkSize 设置分块大小
func (c *ChunkingStrategyAdapter) SetChunkSize(size int) {
	c.strategy.SetChunkSize(size)
}

// SetChunkOverlap 设置分块重叠大小
func (c *ChunkingStrategyAdapter) SetChunkOverlap(overlap int) {
	c.strategy.SetChunkOverlap(overlap)
}
