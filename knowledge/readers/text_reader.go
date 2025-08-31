package readers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/knowledge"
)

// TextFileReader 文本文件读取器
// 从文本文件中读取文档内容
type TextFileReader struct {
	// 文件路径列表
	FilePaths []string
}

// NewTextFileReader 创建文本文件读取器
func NewTextFileReader(filePaths []string) *TextFileReader {
	return &TextFileReader{
		FilePaths: filePaths,
	}
}

// ReadDocuments 从文本文件读取文档
func (tr *TextFileReader) ReadDocuments(ctx context.Context) ([]knowledge.Document, error) {
	var documents []knowledge.Document

	for _, filePath := range tr.FilePaths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取文件 %s 失败: %w", filePath, err)
		}

		now := time.Now()
		doc := knowledge.Document{
			ID:        generateDocumentID(filePath),
			Content:   string(content),
			CreatedAt: now,
			UpdatedAt: now,
			Metadata: map[string]interface{}{
				"source":   filePath,
				"type":     "text",
				"filename": filepath.Base(filePath),
			},
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

// URLReader URL内容读取器
// 从URL获取内容并转换为文档
type URLReader struct {
	// URL列表
	URLs []string
}

// NewURLReader 创建URL读取器
func NewURLReader(urls []string) *URLReader {
	return &URLReader{
		URLs: urls,
	}
}

// ReadDocuments 从URL读取文档
func (ur *URLReader) ReadDocuments(ctx context.Context) ([]knowledge.Document, error) {
	var documents []knowledge.Document

	client := &http.Client{}

	for _, url := range ur.URLs {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("为 %s 创建请求失败: %w", url, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("获取URL %s 失败: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d for URL %s", resp.StatusCode, url)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("从 %s 读取内容失败: %w", url, err)
		}

		now := time.Now()
		doc := knowledge.Document{
			ID:        generateDocumentID(url),
			Content:   string(content),
			CreatedAt: now,
			UpdatedAt: now,
			Metadata: map[string]interface{}{
				"source": url,
				"type":   "url",
			},
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

// DirectoryReader 目录读取器
// 递归读取目录中的文件内容
type DirectoryReader struct {
	// 目录路径
	DirectoryPath string
	// 文件扩展名过滤器（如 ".txt", ".md"）
	Extensions []string
	// 是否递归搜索
	Recursive bool
}

// NewDirectoryReader 创建目录读取器
func NewDirectoryReader(directoryPath string, extensions []string, recursive bool) *DirectoryReader {
	// 默认扩展名
	if len(extensions) == 0 {
		extensions = []string{".txt", ".md", ".markdown"}
	}

	return &DirectoryReader{
		DirectoryPath: directoryPath,
		Extensions:    extensions,
		Recursive:     recursive,
	}
}

// ReadDocuments 从目录读取文档
func (dr *DirectoryReader) ReadDocuments(ctx context.Context) ([]knowledge.Document, error) {
	var documents []knowledge.Document
	var filePaths []string

	if dr.Recursive {
		err := filepath.Walk(dr.DirectoryPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && dr.hasValidExtension(path) {
				filePaths = append(filePaths, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("遍历目录 %s 失败: %w", dr.DirectoryPath, err)
		}
	} else {
		entries, err := os.ReadDir(dr.DirectoryPath)
		if err != nil {
			return nil, fmt.Errorf("读取目录 %s 失败: %w", dr.DirectoryPath, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(dr.DirectoryPath, entry.Name())
				if dr.hasValidExtension(path) {
					filePaths = append(filePaths, path)
				}
			}
		}
	}

	// 读取每个文件
	for _, filePath := range filePaths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("警告: 读取文件 %s 失败: %v", filePath, err)
			continue
		}

		relPath, _ := filepath.Rel(dr.DirectoryPath, filePath)
		now := time.Now()

		doc := knowledge.Document{
			ID:        generateDocumentID(filePath),
			Content:   string(content),
			CreatedAt: now,
			UpdatedAt: now,
			Metadata: map[string]interface{}{
				"source":        filePath,
				"relative_path": relPath,
				"type":          "file",
				"filename":      filepath.Base(filePath),
				"extension":     filepath.Ext(filePath),
				"directory":     dr.DirectoryPath,
			},
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

// hasValidExtension 检查文件是否有有效扩展名
func (dr *DirectoryReader) hasValidExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, validExt := range dr.Extensions {
		if ext == strings.ToLower(validExt) {
			return true
		}
	}
	return false
}

// InMemoryReader 内存文档读取器
// 从内存中的文档列表读取
type InMemoryReader struct {
	// 文档列表
	Documents []knowledge.Document
}

// NewInMemoryReader 创建内存文档读取器
func NewInMemoryReader(documents []knowledge.Document) *InMemoryReader {
	return &InMemoryReader{
		Documents: documents,
	}
}

// ReadDocuments 从内存读取文档
func (ir *InMemoryReader) ReadDocuments(ctx context.Context) ([]knowledge.Document, error) {
	// 返回文档的副本以避免修改
	docs := make([]knowledge.Document, len(ir.Documents))
	copy(docs, ir.Documents)
	return docs, nil
}

// generateDocumentID 生成文档ID的辅助函数
func generateDocumentID(source string) string {
	// 基于来源的简单ID生成
	// 在实际实现中，你可能想使用合适的哈希函数
	return fmt.Sprintf("doc_%x", []byte(source))
}
