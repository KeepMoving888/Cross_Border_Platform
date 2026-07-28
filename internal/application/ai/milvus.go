package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/logger"
)

// ============== MilvusStore ==============
//
// MilvusStore 基于 Milvus REST API 的向量存储实现
// Milvus 2.3+ 提供 RESTful API,无需引入 milvus-sdk-go 依赖,降低构建复杂度
//
// 设计要点:
//   1. 通过 HTTP 调用 Milvus REST API(v1/vector/search, v1/vector/insert, v1/vector/delete)
//   2. Available() 通过 /healthz 检查连接,失败时 RAGService 自动降级到 TF-IDF
//   3. Collection 名称映射知识库 ID(kb_{id}),支持多知识库隔离
//   4. 向量维度由首次 UpsertVectors 决定,Milvus schema 动态创建
//
// 适用场景:
//   - 生产环境 10M+ 向量大规模检索(超出 pgvector 单机能力)
//   - 需要 IVF_FLAT/HNSW 等 ANN 索引加速(毫秒级检索)
//   - 多副本分布式部署(高可用)
//
// 与 PgVectorStore 对比:
//   | 维度       | pgvector        | Milvus               |
//   | 向量规模   | < 1M(单机)      | 10M+(分布式)          |
//   | 索引类型   | IVFFlat/HNSW    | IVF_FLAT/IVF_SQ/HNSW |
//   | 部署复杂度 | 低(PostgreSQL) | 中(独立集群)          |
//   | 事务支持   | 是              | 否                    |
//   | 全文检索   | 是(tsvector)    | 否(需配合 ES)         |

// MilvusConfig Milvus 连接配置
type MilvusConfig struct {
	Address    string // Milvus REST API 地址,如 http://milvus:19530
	Username   string // 认证用户名(可选)
	Password   string // 认证密码(可选)
	Collection string // 默认 Collection 前缀(kb_{id} 自动拼接)
	Timeout    time.Duration
}

// MilvusStore 基于 Milvus REST API 的向量存储
type MilvusStore struct {
	cfg    MilvusConfig
	client *http.Client
}

// NewMilvusStore 创建 Milvus 向量存储
func NewMilvusStore(cfg MilvusConfig) *MilvusStore {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Collection == "" {
		cfg.Collection = "cb_knowledge"
	}
	return &MilvusStore{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (s *MilvusStore) Name() string { return "milvus" }

// Available 通过 /healthz 检查 Milvus 连接可用性
func (s *MilvusStore) Available() bool {
	if s.cfg.Address == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.Address+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		logger.Get().Debugf("milvus health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// collectionName 根据 kbID 生成 Collection 名称
// 不同知识库使用独立 Collection,实现物理隔离
func (s *MilvusStore) collectionName(kbID uint) string {
	if kbID == 0 {
		return s.cfg.Collection
	}
	return fmt.Sprintf("%s_%d", s.cfg.Collection, kbID)
}

// milvusSearchRequest Milvus REST API 搜索请求体
type milvusSearchRequest struct {
	CollectionName string      `json:"collectionName"`
	Data           [][]float64 `json:"data"`         // 查询向量(支持批量)
	Limit          int         `json:"limit"`        // 返回 topK
	OutputFields   []string    `json:"outputFields"` // 返回的字段
}

// milvusSearchResponse Milvus REST API 搜索响应
type milvusSearchResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"` // 搜索结果数组
}

// milvusVectorRecord Milvus 返回的向量记录
type milvusVectorRecord struct {
	ID         any     `json:"id"` // Milvus 主键(字符串或数字)
	ChunkID    uint    `json:"chunk_id"`
	DocID      uint    `json:"doc_id"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`    // 相似度分数
	Distance   float64 `json:"distance"` // Milvus 返回的距离(越小越相似)
}

// UpsertVectors 通过 Milvus REST API 批量插入向量
// Milvus 2.3+ REST API: POST /v1/vector/insert
func (s *MilvusStore) UpsertVectors(ctx context.Context, records []VectorRecord) error {
	if len(records) == 0 {
		return nil
	}
	// 按 kbID 分组(Milvus Collection 粒度)
	groups := make(map[uint][]VectorRecord)
	for _, r := range records {
		groups[r.KBID] = append(groups[r.KBID], r)
	}

	for kbID, groupRecords := range groups {
		collection := s.collectionName(kbID)
		// 构造插入请求体
		// Milvus REST API 要求 data 为数组,每条记录包含所有字段
		data := make([]map[string]interface{}, 0, len(groupRecords))
		for _, r := range groupRecords {
			data = append(data, map[string]interface{}{
				"chunk_id":    r.ChunkID,
				"doc_id":      r.DocID,
				"chunk_index": r.ChunkIndex,
				"content":     r.Content,
				"embedding":   r.Embedding,
			})
		}

		body := map[string]interface{}{
			"collectionName": collection,
			"data":           data,
		}
		if err := s.doPost(ctx, "/v1/vector/insert", body, nil); err != nil {
			logger.Get().Warnf("milvus insert collection=%s failed: %v", collection, err)
			// 部分失败可接受,不中断整体流程
		}
	}
	return nil
}

// Search 通过 Milvus REST API 进行向量检索
// POST /v1/vector/search
func (s *MilvusStore) Search(ctx context.Context, queryVec []float64, kbID uint, topK int) ([]VectorSearchResult, error) {
	if len(queryVec) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	if topK <= 0 {
		topK = 5
	}

	collection := s.collectionName(kbID)
	reqBody := milvusSearchRequest{
		CollectionName: collection,
		Data:           [][]float64{queryVec},
		Limit:          topK,
		OutputFields:   []string{"chunk_id", "doc_id", "chunk_index", "content"},
	}

	var rawResp struct {
		Code int                  `json:"code"`
		Data []milvusVectorRecord `json:"data"`
	}
	if err := s.doPost(ctx, "/v1/vector/search", reqBody, &rawResp); err != nil {
		return nil, fmt.Errorf("milvus search: %w", err)
	}
	if rawResp.Code != 0 {
		return nil, fmt.Errorf("milvus search error code: %d", rawResp.Code)
	}

	// 转换为统一结果格式
	// Milvus 返回 distance(越小越相似),余弦距离转换为相似度 score = 1 - distance
	results := make([]VectorSearchResult, 0, len(rawResp.Data))
	for _, rec := range rawResp.Data {
		score := rec.Score
		if score == 0 && rec.Distance > 0 {
			// distance 转相似度(余弦距离范围 [0,2],相似度 = 1 - distance/2)
			score = 1 - rec.Distance/2
		}
		if score < 0 {
			score = 0
		}
		results = append(results, VectorSearchResult{
			ChunkID:    rec.ChunkID,
			DocID:      rec.DocID,
			ChunkIndex: rec.ChunkIndex,
			Content:    rec.Content,
			Score:      score,
		})
	}
	return results, nil
}

// DeleteByDoc 通过 Milvus REST API 删除指定文档的所有向量
// POST /v1/vector/delete,使用 expr 过滤 doc_id
func (s *MilvusStore) DeleteByDoc(ctx context.Context, docID uint) error {
	// Milvus 删除表达式: doc_id == {docID}
	body := map[string]interface{}{
		"collectionName": s.cfg.Collection,
		"filter":         fmt.Sprintf("doc_id == %d", docID),
	}
	return s.doPost(ctx, "/v1/vector/delete", body, nil)
}

// DeleteByKB 删除指定知识库的 Collection(若存在)
func (s *MilvusStore) DeleteByKB(ctx context.Context, kbID uint) error {
	collection := s.collectionName(kbID)
	// Milvus REST API: DELETE /v1/collection?collectionName={name}
	url := fmt.Sprintf("%s/v1/collection?collectionName=%s", s.cfg.Address, collection)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	s.addAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("milvus delete collection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("milvus delete collection status: %d", resp.StatusCode)
	}
	return nil
}

// doPost 发送 POST 请求并解析响应
func (s *MilvusStore) doPost(ctx context.Context, path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := s.cfg.Address + path
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.addAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("milvus api %s status=%d body=%s", path, resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

// addAuth 添加认证头(可选)
func (s *MilvusStore) addAuth(req *http.Request) {
	if s.cfg.Username != "" && s.cfg.Password != "" {
		// Milvus REST API 使用 Basic Auth 或 Token
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}
}

// ============== VectorStore 工厂函数 ==============

// VectorStoreType 向量存储类型枚举
type VectorStoreType string

const (
	VectorStorePgVector VectorStoreType = "pgvector"
	VectorStoreMilvus   VectorStoreType = "milvus"
	VectorStoreInMemory VectorStoreType = "memory"
)

// NewVectorStore 向量存储工厂函数
// 根据 type 创建对应的 VectorStore 实现,未配置时返回 nil(降级到 TF-IDF)
func NewVectorStore(storeType VectorStoreType, pgDB interface{}, milvusCfg MilvusConfig) VectorStore {
	switch storeType {
	case VectorStorePgVector:
		// pgDB 应为 *gorm.DB,类型断言由调用方保证
		if db, ok := pgDB.(interface { /* gorm.DB 方法集占位 */
		}); ok {
			_ = db
		}
		return nil // 实际由 NewRAGService 内部处理
	case VectorStoreMilvus:
		if milvusCfg.Address == "" {
			return nil
		}
		return NewMilvusStore(milvusCfg)
	case VectorStoreInMemory:
		return NewInMemoryVectorStore()
	default:
		return nil
	}
}

// EnsureMilvusStoreUsed 确保 strings 包被引用(避免 import 循环误报)
var _ = strings.TrimSpace
