package collector

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/server-probe/shared/model"
)

// clkTck 是 Linux 系统的时钟滴答频率（USER_HZ）。
// 绝大多数 Linux 系统为 100 Hz。/proc/[pid]/stat 中 utime/stime 的单位为时钟滴答数。
const clkTck = 100

// processCPUState 记录进程上一次采集时的 CPU 时间状态
type processCPUState struct {
	totalTime uint64    // utime + stime（时钟滴答数）
	timestamp time.Time // 采集时刻
}

// ProcessCollector 进程列表采集器
// 采集 Top N 进程（按内存 RSS 排序），包含 CPU 使用率和内存信息
type ProcessCollector struct {
	reader     FileReader
	pageSize   int
	prevStates map[int]processCPUState // PID -> 上次 CPU 状态
	prevTime   time.Time               // 上次采集时刻
	topN       int                     // 返回的进程数量上限
}

// NewProcessCollector 创建进程列表采集器
func NewProcessCollector(reader FileReader) *ProcessCollector {
	return &ProcessCollector{
		reader:     reader,
		pageSize:   os.Getpagesize(),
		prevStates: make(map[int]processCPUState),
		topN:       10,
	}
}

// Name 返回采集器名称
func (c *ProcessCollector) Name() string {
	return "process"
}

// Collect 采集 Top N 进程列表（按 RSS 排序）
func (c *ProcessCollector) Collect() (interface{}, error) {
	// 列出 /proc 目录中的所有 PID
	pids, err := c.listPIDs()
	if err != nil {
		return []model.ProcessInfo{}, fmt.Errorf("列出进程失败: %w", err)
	}

	now := time.Now()
	var elapsed float64
	if !c.prevTime.IsZero() {
		elapsed = now.Sub(c.prevTime).Seconds()
	}

	// 在循环外计算 CPU 核心数，避免每次迭代重复调用 runtime.NumCPU()
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}

	// 采集每个进程的信息
	newStates := make(map[int]processCPUState, len(pids))
	processes := make([]model.ProcessInfo, 0, len(pids))

	for _, pid := range pids {
		info, state, ok := c.collectProcess(pid, elapsed, cores)
		if !ok {
			continue
		}
		processes = append(processes, info)
		if state != nil {
			newStates[pid] = *state
		}
	}

	// 更新状态：替换为本次采集到的进程状态，自动清除已退出的进程
	c.prevStates = newStates
	c.prevTime = now

	// 按 RSS 降序排序
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].RSS > processes[j].RSS
	})

	// 截取 Top N
	if len(processes) > c.topN {
		processes = processes[:c.topN]
	}

	return processes, nil
}

// collectProcess 采集单个进程的信息
// 返回 (进程信息, CPU状态, 是否成功)
// 单个进程读取失败不影响其他进程
// cores 为 CPU 核心数，由调用方在循环外计算一次后传入
func (c *ProcessCollector) collectProcess(pid int, elapsed float64, cores int) (model.ProcessInfo, *processCPUState, bool) {
	info := model.ProcessInfo{PID: pid}

	// 1. 读取进程名 /proc/[pid]/comm
	commData, err := c.reader.ReadFile(fmt.Sprintf("%s/%d/comm", ProcPath, pid))
	if err != nil {
		return info, nil, false
	}
	info.Name = strings.TrimSpace(string(commData))

	// 2. 读取 /proc/[pid]/statm 获取内存信息
	// 格式: size resident shared text lib data dt（单位：页）
	statmData, err := c.reader.ReadFile(fmt.Sprintf("%s/%d/statm", ProcPath, pid))
	if err != nil {
		return info, nil, false
	}

	virtMem, rss, ok := parseStatm(string(statmData))
	if !ok {
		return info, nil, false
	}

	// 转换为字节
	info.Memory = virtMem * uint64(c.pageSize)
	info.RSS = rss * uint64(c.pageSize)

	// 3. 读取 /proc/[pid]/stat 获取 CPU 时间
	statData, err := c.reader.ReadFile(fmt.Sprintf("%s/%d/stat", ProcPath, pid))
	if err != nil {
		// 无法读取 stat，CPU 使用率设为 0
		info.CPU = 0
		return info, nil, true
	}

	utime, stime, ok := parseProcStat(string(statData))
	if !ok {
		info.CPU = 0
		return info, nil, true
	}

	currentTotal := utime + stime

	// 计算 CPU 使用率
	// 第一次调用（无前次数据）或 elapsed <= 0 时返回 0
	info.CPU = 0
	var state *processCPUState
	if prev, exists := c.prevStates[pid]; exists && elapsed > 0 {
		if currentTotal > prev.totalTime {
			delta := currentTotal - prev.totalTime
			// CPU 使用率 = (CPU时间增量 / 时钟频率) / 经过时间 * 100 / CPU核心数
			// 归一化到 0-100%（占总 CPU 容量的百分比）
			info.CPU = roundFloat(float64(delta)/(float64(clkTck)*elapsed*float64(cores))*100, 2)
		}
	}

	state = &processCPUState{
		totalTime: currentTotal,
		timestamp: time.Now(),
	}

	return info, state, true
}

// listPIDs 列出 /proc 目录中所有以数字命名的目录（即 PID）
func (c *ProcessCollector) listPIDs() ([]int, error) {
	entries, err := c.reader.ReadDir(ProcPath)
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}

	return pids, nil
}

// parseStatm 解析 /proc/[pid]/statm
// 格式: size resident shared text lib data dt
// 返回虚拟内存大小（页）和 RSS（页）
func parseStatm(data string) (virtMem uint64, rss uint64, ok bool) {
	fields := strings.Fields(data)
	if len(fields) < 2 {
		return 0, 0, false
	}

	virtMem, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	rss, err = strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return virtMem, rss, true
}

// parseProcStat 解析 /proc/[pid]/stat
// 格式: pid (comm) state ppid ... utime stime ...
// comm 字段在括号内，可能包含空格，因此需要找到最后一个 ')' 来正确分割字段
// utime 是第 14 个字段，stime 是第 15 个字段（从 1 开始计数）
func parseProcStat(data string) (utime uint64, stime uint64, ok bool) {
	// 找到最后一个 ')' 以跳过 comm 字段
	lastParen := strings.LastIndex(data, ")")
	if lastParen == -1 {
		return 0, 0, false
	}

	// 剩余部分按空格分割
	rest := strings.Fields(data[lastParen+1:])
	// rest[0] = state (field 3)
	// rest[1] = ppid  (field 4)
	// ...
	// rest[11] = utime (field 14)
	// rest[12] = stime (field 15)
	if len(rest) < 13 {
		return 0, 0, false
	}

	var err error
	utime, err = strconv.ParseUint(rest[11], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	stime, err = strconv.ParseUint(rest[12], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return utime, stime, true
}
