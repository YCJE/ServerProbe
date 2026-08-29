package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
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

	// 注册码必须关联显示名称，防止空名 Agent 混入列表
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "显示名称不能为空"})
		return
	}

	// 校验输入长度，防止超长字符串写入数据库
	if len(req.DisplayName) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "显示名称过长（最多 100 字符）"})
		return
	}
	if len(req.Remark) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "备注过长（最多 500 字符）"})
		return
	}

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

// HandleCreateAgent 直接创建 Agent（Komari 风格）
// 路由: POST /api/v1/agents
// 后台先添加基本信息创建记录并生成 Token，随后前端展示一键安装命令（携带 Token），
// 被监控服务器执行命令后 Agent 用 Token 首次连接并回填主机信息
func (h *AgentAPIHandler) HandleCreateAgent(c *gin.Context) {
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入服务器名称"})
		return
	}
	if len(displayName) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "显示名称过长（最多 100 字符）"})
		return
	}

	agent, err := h.registry.CreateAgent(displayName)
	if err != nil {
		log.Printf("创建 Agent 失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Agent 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agent_id":     agent.ID,
		"display_name": agent.DisplayName,
		"token":        agent.Token,
	})
}

// HandleGetAgentToken 获取 Agent Token（用于生成重装命令）
// 路由: GET /api/v1/agents/:id/token
// Token 仅在管理员认证下返回，用于为已存在的 Agent 重新生成一键安装命令（重装/换机场景）
func (h *AgentAPIHandler) HandleGetAgentToken(c *gin.Context) {
	id := c.Param("id")
	agentID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	agent, err := h.agentRepo.GetByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent 不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agent_id":     agent.ID,
		"display_name": agent.DisplayName,
		"token":        agent.Token,
	})
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
		Tags        string `json:"tags"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if len(req.DisplayName) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "显示名称过长"})
		return
	}
	if len(req.Tags) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标签过长（最多 500 字符）"})
		return
	}

	// 原子更新 display_name 与 tags 字段（单条 SQL，避免部分更新）
	if err := h.agentRepo.UpdateProfile(agentID, req.DisplayName, req.Tags); err != nil {
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

// HandleUpdateAgentMeta 更新 Agent NodeGet 风格元数据（位置/国旗/供应商/到期/费用/流量配额）
// 路由: PUT /api/v1/agents/:id/meta
func (h *AgentAPIHandler) HandleUpdateAgentMeta(c *gin.Context) {
	id := c.Param("id")
	agentID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	var req struct {
		Region            string  `json:"region"`
		CountryCode       string  `json:"country_code"`
		ISP               string  `json:"isp"`
		ExpiresAt         string  `json:"expires_at"` // RFC3339 或 "YYYY-MM-DD"，空字符串表示永不过期
		PriceAmount       float64 `json:"price_amount"`
		PriceCurrency     string  `json:"price_currency"`
		PriceCycle        string  `json:"price_cycle"`
		TrafficQuotaBytes int64   `json:"traffic_quota_bytes"`
		TrafficQuotaType  string  `json:"traffic_quota_type"` // sum/up/down/max/min，空=默认 sum
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	// 输入校验：防止超长字符串与非法枚举值写入数据库
	if len(req.Region) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "位置过长（最多 100 字符）"})
		return
	}
	req.CountryCode = strings.ToUpper(strings.TrimSpace(req.CountryCode))
	if len(req.CountryCode) > 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "国家代码须为 2 位字母（如 CN/US/JP）"})
		return
	}
	if len(req.ISP) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "供应商过长（最多 100 字符）"})
		return
	}
	switch req.PriceCurrency {
	case "", "CNY", "USD", "EUR", "JPY", "GBP", "HKD", "KRW", "SGD":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的币种"})
		return
	}
	switch req.PriceCycle {
	case "", "monthly", "yearly", "quarterly", "weekly":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "周期须为 monthly/quarterly/yearly/weekly"})
		return
	}
	if req.PriceAmount < 0 || req.PriceAmount > 1e9 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "费用数值无效"})
		return
	}
	if req.TrafficQuotaBytes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "流量配额无效"})
		return
	}
	// 配额口径白名单校验（空=默认 sum，与存量数据行为一致）
	req.TrafficQuotaType = strings.TrimSpace(req.TrafficQuotaType)
	if req.TrafficQuotaType == "" {
		req.TrafficQuotaType = model.QuotaTypeSum
	} else if !model.ValidQuotaTypes[req.TrafficQuotaType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "流量配额口径须为 sum/up/down/max/min"})
		return
	}

	// 解析到期时间：空 = 永不过期（ExpiresAt 为 NULL）
	var expiresAt *time.Time
	if s := strings.TrimSpace(req.ExpiresAt); s != "" {
		t, err := parseFlexibleDate(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "到期时间格式无效（支持 RFC3339 或 YYYY-MM-DD）"})
			return
		}
		expiresAt = &t
	}

	meta := repository.AgentMeta{
		Region:            strings.TrimSpace(req.Region),
		CountryCode:       req.CountryCode,
		ISP:               strings.TrimSpace(req.ISP),
		ExpiresAt:         expiresAt,
		PriceAmount:       req.PriceAmount,
		PriceCurrency:     req.PriceCurrency,
		PriceCycle:        req.PriceCycle,
		TrafficQuotaBytes: req.TrafficQuotaBytes,
		TrafficQuotaType:  req.TrafficQuotaType,
	}

	if err := h.agentRepo.UpdateMeta(agentID, meta); err != nil {
		log.Printf("更新 Agent %d 元数据失败: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新元数据失败"})
		return
	}

	agent, err := h.agentRepo.GetByID(agentID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "agent": agent})
}

// parseFlexibleDate 解析 RFC3339 或 YYYY-MM-DD 格式的时间字符串
func parseFlexibleDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", s)
}
