package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// AgentAPIHandler Agent 管理 API 处理器 (面向前端)
type AgentAPIHandler struct {
	registry   *service.AgentRegistryService
	agentRepo  *repository.AgentRepository
	recordRepo *repository.RecordRepository
	monitor    *service.MonitorService
	engine     *service.AlertEngine
}

// NewAgentAPIHandler 创建 Agent API 处理器
func NewAgentAPIHandler(registry *service.AgentRegistryService, agentRepo *repository.AgentRepository, recordRepo *repository.RecordRepository, monitor *service.MonitorService, engine *service.AlertEngine) *AgentAPIHandler {
	return &AgentAPIHandler{
		registry:   registry,
		agentRepo:  agentRepo,
		recordRepo: recordRepo,
		monitor:    monitor,
		engine:     engine,
	}
}

// RegisterCodeResponse 注册码响应
type RegisterCodeResponse struct {
	Code        string    `json:"code"`
	DisplayName string    `json:"display_name"`
	Remark      string    `json:"remark"`
	ExpiresAt   time.Time `json:"expires_at"`
	Used        bool      `json:"used"`
}

// HandleGenerateRegisterCode 生成注册码
// 路由: POST /api/v1/agents/register-codes
func (h *AgentAPIHandler) HandleGenerateRegisterCode(c *gin.Context) {
	var req struct {
		DisplayName string `json:"display_name"`
		Remark      string `json:"remark"`
	}
	// 忽略绑定错误，允许空 body
	_ = c.ShouldBindJSON(&req)

	rc, err := h.registry.GenerateRegisterCode(req.DisplayName, req.Remark)
	if err != nil {
		log.Printf("生成注册码失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, RegisterCodeResponse{
		Code:        rc.Code,
		DisplayName: rc.DisplayName,
		Remark:      rc.Remark,
		ExpiresAt:   rc.ExpiresAt,
		Used:        rc.Used,
	})
}

// HandleListRegisterCodes 列出所有未使用的注册码
// 路由: GET /api/v1/agents/register-codes
func (h *AgentAPIHandler) HandleListRegisterCodes(c *gin.Context) {
	codes, err := h.registry.ListRegisterCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取注册码列表失败"})
		return
	}

	result := make([]RegisterCodeResponse, 0, len(codes))
	for _, rc := range codes {
		result = append(result, RegisterCodeResponse{
			Code:        rc.Code,
			DisplayName: rc.DisplayName,
			Remark:      rc.Remark,
			ExpiresAt:   rc.ExpiresAt,
			Used:        rc.Used,
		})
	}

	c.JSON(http.StatusOK, gin.H{"codes": result})
}

// HandleDeleteRegisterCode 删除注册码
// 路由: DELETE /api/v1/agents/register-codes/:code
func (h *AgentAPIHandler) HandleDeleteRegisterCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少注册码"})
		return
	}

	if err := h.registry.DeleteRegisterCode(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除注册码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleListAgents 列出所有 Agent
// 路由: GET /api/v1/agents
func (h *AgentAPIHandler) HandleListAgents(c *gin.Context) {
	agents, err := h.agentRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Agent 列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// HandleDeleteAgent 删除 Agent
// 路由: DELETE /api/v1/agents/:id
func (h *AgentAPIHandler) HandleDeleteAgent(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 Agent ID"})
		return
	}

	agentID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	// 先删除关联的历史聚合数据和 Agent 记录 (使用事务确保原子性)
	// 先执行 DB 删除，成功后再清理内存状态，避免 DB 删除失败但内存已被清理
	if err := h.agentRepo.DeleteWithRecordsTx(agentID); err != nil {
		log.Printf("删除 Agent %d 及其历史数据失败: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除 Agent 失败"})
		return
	}

	// DB 删除成功后，清理 MonitorService 中的连接和 ringBuffer
	h.monitor.UnregisterAgent(agentID)

	// 清理告警引擎中的状态
	if h.engine != nil {
		h.engine.CleanupStatesForAgent(agentID)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleUpdateAgent 更新 Agent 信息 (显示名称)
// 路由: PUT /api/v1/agents/:id
func (h *AgentAPIHandler) HandleUpdateAgent(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 Agent ID"})
		return
	}

	agentID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if len(req.DisplayName) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "显示名称过长"})
		return
	}

	// 仅更新 display_name 字段，避免 Save 覆盖 Online/LastSeen 等并发修改的字段
	if err := h.agentRepo.UpdateDisplayName(agentID, req.DisplayName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 Agent 失败"})
		return
	}

	// 重新查询返回更新后的完整 Agent 信息
	agent, err := h.agentRepo.GetByID(agentID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "agent": agent})
}
