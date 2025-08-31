package chunking

import (
	"fmt"
	"strings"
	"time"
)

// Document 文档结构（避免循环导入）
type Document struct {
	ID        string
	Content   string
	Metadata  map[string]interface{}
	Vector    []float32
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Chunk 文档分块结构
type Chunk struct {
	ID          string
	DocumentID  string
	Content     string
	Metadata    map[string]interface{}
	Vector      []float32
	Index       int
	StartOffset int
	EndOffset   int
}

// FixedSizeChunkingStrategy 固定大小分块策略
// 按照固定的字符数将文档分割成块
type FixedSizeChunkingStrategy struct {
	// 分块大小（字符数）
	chunkSize int
	// 分块重叠大小（字符数）
	chunkOverlap int
}

// NewFixedSizeChunkingStrategy 创建固定大小分块策略
func NewFixedSizeChunkingStrategy(chunkSize, chunkOverlap int) *FixedSizeChunkingStrategy {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 2
	}

	return &FixedSizeChunkingStrategy{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

// Chunk 将文档分割成块
func (fs *FixedSizeChunkingStrategy) Chunk(doc Document) ([]Chunk, error) {
	if doc.Content == "" {
		return []Chunk{}, nil
	}

	var chunks []Chunk
	content := doc.Content
	contentLen := len([]rune(content))

	if contentLen <= fs.chunkSize {
		// 文档小于分块大小，直接返回一个块
		chunk := Chunk{
			ID:          fmt.Sprintf("%s_chunk_0", doc.ID),
			DocumentID:  doc.ID,
			Content:     content,
			Index:       0,
			StartOffset: 0,
			EndOffset:   contentLen,
			Metadata:    make(map[string]interface{}),
		}

		// 复制原文档的元数据
		for k, v := range doc.Metadata {
			chunk.Metadata[k] = v
		}

		return []Chunk{chunk}, nil
	}

	// 将内容转换为rune数组以正确处理Unicode字符
	runes := []rune(content)
	index := 0
	start := 0

	for start < contentLen {
		end := start + fs.chunkSize
		if end > contentLen {
			end = contentLen
		}

		// 尝试在自然边界处分割（避免在单词中间分割）
		if end < contentLen {
			end = fs.findNaturalBreakpoint(runes, start, end)
		}

		chunkContent := string(runes[start:end])
		chunk := Chunk{
			ID:          fmt.Sprintf("%s_chunk_%d", doc.ID, index),
			DocumentID:  doc.ID,
			Content:     chunkContent,
			Index:       index,
			StartOffset: start,
			EndOffset:   end,
			Metadata:    make(map[string]interface{}),
		}

		// 复制原文档的元数据
		for k, v := range doc.Metadata {
			chunk.Metadata[k] = v
		}

		chunks = append(chunks, chunk)

		// 计算下一个块的起始位置（考虑重叠）
		start = end - fs.chunkOverlap
		if start < 0 {
			start = 0
		}

		index++
	}

	return chunks, nil
}

// findNaturalBreakpoint 寻找自然的分割点
// 优先选择句号、换行符、空格等作为分割点
func (fs *FixedSizeChunkingStrategy) findNaturalBreakpoint(runes []rune, start, end int) int {
	// 在最后200个字符中寻找自然分割点
	searchStart := end - 200
	if searchStart < start {
		searchStart = start
	}

	// 优先级：句号 > 换行符 > 感叹号/问号 > 逗号/分号 > 空格
	breakpoints := [][]rune{
		{'.', '\n'},
		{'\n'},
		{'!', '?'},
		{',', ';', ':'},
		{' ', '\t'},
	}

	for _, breaks := range breakpoints {
		for i := end - 1; i >= searchStart; i-- {
			for _, breakChar := range breaks {
				if runes[i] == breakChar {
					// 确保不返回起始位置
					if i > start {
						return i + 1
					}
				}
			}
		}
	}

	// 如果没有找到合适的分割点，使用原始结束位置
	return end
}

// GetChunkSize 获取分块大小
func (fs *FixedSizeChunkingStrategy) GetChunkSize() int {
	return fs.chunkSize
}

// GetChunkOverlap 获取分块重叠大小
func (fs *FixedSizeChunkingStrategy) GetChunkOverlap() int {
	return fs.chunkOverlap
}

// SetChunkSize 设置分块大小
func (fs *FixedSizeChunkingStrategy) SetChunkSize(size int) {
	if size > 0 {
		fs.chunkSize = size
		// 确保重叠大小不超过分块大小的一半
		if fs.chunkOverlap >= size {
			fs.chunkOverlap = size / 2
		}
	}
}

// SetChunkOverlap 设置分块重叠大小
func (fs *FixedSizeChunkingStrategy) SetChunkOverlap(overlap int) {
	if overlap >= 0 && overlap < fs.chunkSize {
		fs.chunkOverlap = overlap
	}
}

// SentenceChunkingStrategy 句子分块策略
// 按照句子边界将文档分割成块
type SentenceChunkingStrategy struct {
	// 最大分块大小（字符数）
	maxChunkSize int
	// 分块重叠句子数
	overlapSentences int
}

// NewSentenceChunkingStrategy 创建句子分块策略
func NewSentenceChunkingStrategy(maxChunkSize, overlapSentences int) *SentenceChunkingStrategy {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000
	}
	if overlapSentences < 0 {
		overlapSentences = 1
	}

	return &SentenceChunkingStrategy{
		maxChunkSize:     maxChunkSize,
		overlapSentences: overlapSentences,
	}
}

// Chunk 将文档按句子分割成块
func (ss *SentenceChunkingStrategy) Chunk(doc Document) ([]Chunk, error) {
	if doc.Content == "" {
		return []Chunk{}, nil
	}

	// 分割句子
	sentences := ss.splitIntoSentences(doc.Content)
	if len(sentences) == 0 {
		return []Chunk{}, nil
	}

	var chunks []Chunk
	var currentChunk []string
	var currentSize int
	index := 0
	startSentenceIndex := 0

	for i, sentence := range sentences {
		sentenceLen := len([]rune(sentence))

		// 如果当前句子加上现有块会超过最大大小，则完成当前块
		if currentSize+sentenceLen > ss.maxChunkSize && len(currentChunk) > 0 {
			chunk := ss.createChunk(doc, currentChunk, index, startSentenceIndex, i-1)
			chunks = append(chunks, chunk)
			index++

			// 开始新块时保留一些重叠句子
			overlapStart := len(currentChunk) - ss.overlapSentences
			if overlapStart < 0 {
				overlapStart = 0
			}
			currentChunk = currentChunk[overlapStart:]
			currentSize = ss.calculateSize(currentChunk)
			startSentenceIndex = i - len(currentChunk)
		}

		currentChunk = append(currentChunk, sentence)
		currentSize += sentenceLen
	}

	// 处理最后一个块
	if len(currentChunk) > 0 {
		chunk := ss.createChunk(doc, currentChunk, index, startSentenceIndex, len(sentences)-1)
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// splitIntoSentences 将文本分割成句子
func (ss *SentenceChunkingStrategy) splitIntoSentences(text string) []string {
	// 简单的句子分割实现，可以根据需要改进
	sentences := strings.FieldsFunc(text, func(c rune) bool {
		return c == '.' || c == '!' || c == '?' || c == '\n'
	})

	var result []string
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence != "" {
			result = append(result, sentence)
		}
	}

	return result
}

// createChunk 创建分块
func (ss *SentenceChunkingStrategy) createChunk(doc Document, sentences []string, index, startSentenceIndex, endSentenceIndex int) Chunk {
	content := strings.Join(sentences, ". ")
	if content != "" && !strings.HasSuffix(content, ".") {
		content += "."
	}

	chunk := Chunk{
		ID:          fmt.Sprintf("%s_chunk_%d", doc.ID, index),
		DocumentID:  doc.ID,
		Content:     content,
		Index:       index,
		StartOffset: startSentenceIndex,
		EndOffset:   endSentenceIndex + 1,
		Metadata:    make(map[string]interface{}),
	}

	// 复制原文档的元数据
	for k, v := range doc.Metadata {
		chunk.Metadata[k] = v
	}

	chunk.Metadata["sentence_start"] = startSentenceIndex
	chunk.Metadata["sentence_end"] = endSentenceIndex

	return chunk
}

// calculateSize 计算句子列表的总字符数
func (ss *SentenceChunkingStrategy) calculateSize(sentences []string) int {
	total := 0
	for _, sentence := range sentences {
		total += len([]rune(sentence))
	}
	return total
}

// GetChunkSize 获取最大分块大小
func (ss *SentenceChunkingStrategy) GetChunkSize() int {
	return ss.maxChunkSize
}

// GetChunkOverlap 获取重叠句子数
func (ss *SentenceChunkingStrategy) GetChunkOverlap() int {
	return ss.overlapSentences
}

// SetChunkSize 设置最大分块大小
func (ss *SentenceChunkingStrategy) SetChunkSize(size int) {
	if size > 0 {
		ss.maxChunkSize = size
	}
}

// SetChunkOverlap 设置重叠句子数
func (ss *SentenceChunkingStrategy) SetChunkOverlap(overlap int) {
	if overlap >= 0 {
		ss.overlapSentences = overlap
	}
}
