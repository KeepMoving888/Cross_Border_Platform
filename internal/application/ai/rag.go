package ai

import (
	"fmt"
	"strings"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"gorm.io/gorm"
)

// RAGService 简化版 RAG 检索服务
// 基于 TF-IDF 关键词匹配(非向量检索),适合演示和小规模知识库
type RAGService struct {
	db *gorm.DB
}

// NewRAGService 创建 RAG 服务
func NewRAGService(db *gorm.DB) *RAGService {
	return &RAGService{db: db}
}

// RAGDocument RAG 检索结果
type RAGDocument struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Source  string  `json:"source"`
	Score   float64 `json:"score"`
}

// Search 基于关键词检索知识库文档
// query: 用户查询文本
// knowledgeBaseID: 知识库 ID(0 表示搜索所有)
// topK: 返回前 K 条
func (r *RAGService) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	if topK <= 0 {
		topK = 5
	}

	// 分词(简化:按空格和中文字符切分)
	keywords := tokenize(query)
	if len(keywords) == 0 {
		return []RAGDocument{}, nil
	}

	// 查询知识文档
	var docs []models.KnowledgeDocument
	queryDB := r.db.Where("status = ?", "ready")
	if knowledgeBaseID > 0 {
		queryDB = queryDB.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	if err := queryDB.Find(&docs).Error; err != nil {
		return nil, err
	}

	// 计算每个文档的匹配分数(TF 简化版)
	results := make([]RAGDocument, 0, len(docs))
	for _, doc := range docs {
		score := calculateScore(doc.Title+" "+doc.Content, keywords)
		if score > 0 {
			results = append(results, RAGDocument{
				Title:   doc.Title,
				Content: truncate(doc.Content, 500),
				Source:  doc.Source,
				Score:   score,
			})
		}
	}

	// 按分数降序排序,取 topK
	sortDocumentsByScore(results)
	if len(results) > topK {
		results = results[:topK]
	}

	topScore := 0.0
	if len(results) > 0 {
		topScore = results[0].Score
	}
	logger.Get().Infof("RAG search: query=%q, found %d docs (top score: %.2f)", query, len(results), topScore)
	return results, nil
}

// SearchAsContext 检索并格式化为 LLM 上下文字符串
func (r *RAGService) SearchAsContext(query string, knowledgeBaseID uint, topK int) string {
	docs, err := r.Search(query, knowledgeBaseID, topK)
	if err != nil || len(docs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("以下是相关知识库文档片段:\n\n")
	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("【文档 %d】%s (相关度: %.0f%%)\n", i+1, doc.Title, doc.Score*100))
		sb.WriteString(doc.Content)
		sb.WriteString("\n\n---\n\n")
	}
	return sb.String()
}

// tokenize 简单分词
func tokenize(text string) []string {
	// 按空格、标点切分(中文按字切分)
	text = strings.ToLower(text)
	var tokens []string
	// 简化:按非字母数字字符切分
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 0x4e00)
	}) {
		if len(w) >= 2 {
			tokens = append(tokens, w)
		}
	}
	// 中文按 2-gram 切分
	for _, r := range text {
		if r > 0x4e00 {
			// 中文字符,作为单字 token
			tokens = append(tokens, string(r))
		}
	}
	return tokens
}

// calculateScore 计算文档与关键词的匹配分数
func calculateScore(docText string, keywords []string) float64 {
	docLower := strings.ToLower(docText)
	totalScore := 0.0
	for _, kw := range keywords {
		count := strings.Count(docLower, strings.ToLower(kw))
		if count > 0 {
			// TF 简化:出现次数 / 文档长度
			totalScore += float64(count) / float64(len(docLower)+1) * 100
		}
	}
	return totalScore
}

// sortDocumentsByScore 按分数降序排序
func sortDocumentsByScore(docs []RAGDocument) {
	for i := 0; i < len(docs)-1; i++ {
		for j := i + 1; j < len(docs); j++ {
			if docs[i].Score < docs[j].Score {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}
}
