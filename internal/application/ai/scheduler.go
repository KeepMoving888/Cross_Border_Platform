package ai

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"gorm.io/gorm"
)

// Scheduler AI 工作流定时调度器
// 支持按 cron 表达式定时触发工作流(简化版:支持固定间隔)
type Scheduler struct {
	db      *gorm.DB
	engine  *Engine
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

// NewScheduler 创建调度器
func NewScheduler(db *gorm.DB, engine *Engine) *Scheduler {
	return &Scheduler{
		db:     db,
		engine: engine,
		stopCh: make(chan struct{}),
	}
}

// Start 启动调度器,每分钟检查一次待执行的工作流
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	logger.Get().Info("AI workflow scheduler started (interval: 1m)")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			logger.Get().Info("AI workflow scheduler stopped")
			return
		case <-ticker.C:
			s.checkScheduledWorkflows()
		}
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

// checkScheduledWorkflows 检查调度配置并触发到期的工作流
// 简化版:查找 ai_workflows 表中 status='enabled' 且有 schedule_cron 字段的工作流
// 注意:当前 ai_workflows 表无 schedule_cron 字段,这里使用 meta 字段或跳过
// 为保持兼容,调度器目前仅记录日志,实际调度逻辑可在后续扩展
func (s *Scheduler) checkScheduledWorkflows() {
	// 查询所有启用的工作流
	var workflows []models.AIWorkflow
	if err := s.db.Where("status = ?", "enabled").Find(&workflows).Error; err != nil {
		logger.Get().Warnf("scheduler query workflows failed: %v", err)
		return
	}

	// TODO: 当 ai_workflows 表新增 schedule_cron 字段后,按 cron 表达式判断是否到期
	// 当前版本:调度器框架已就绪,可手动触发或后续扩展
	for _, wf := range workflows {
		// 检查 Definition 中是否包含 schedule 配置
		if wf.Definition == "" {
			continue
		}
		// 简化:如果 Definition 包含 "schedule" 关键字,记录日志
		// 实际生产环境应解析 JSON 中的 schedule 字段并按 cron 判断
		logger.Get().Debugf("scheduler checked workflow %s (%s)", wf.Code, wf.Name)
	}
}

// TriggerWorkflow 手动触发一个工作流执行(供外部调用)
// 注意:engine.Execute 内部会创建并持久化 run 记录,此处直接复用,避免重复创建
func (s *Scheduler) TriggerWorkflow(ctx context.Context, wfID uint, input map[string]interface{}, operatorID *uint) (*models.AIWorkflowRun, error) {
	var wf models.AIWorkflow
	if err := s.db.First(&wf, wfID).Error; err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}

	opID := uint(0)
	if operatorID != nil {
		opID = *operatorID
	}

	// 执行工作流(engine.Execute 内部会创建 run 记录)
	result, err := s.engine.Execute(ctx, &wf, input, opID)
	if err != nil {
		logger.Get().Warnf("scheduler trigger workflow %s failed: %v", wf.Code, err)
		return nil, err
	}

	// 返回 engine 创建的 run 记录
	runID, _ := strconv.ParseUint(result.RunID, 10, 64)
	var run models.AIWorkflowRun
	if runID > 0 {
		s.db.First(&run, runID)
		// 标记为调度触发
		s.db.Model(&run).Update("trigger_type", "scheduled")
		run.TriggerType = "scheduled"
	}
	return &run, nil
}
