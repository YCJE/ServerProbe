# P1 实施计划：前端 UI 一致性重构

> **依据**: 三路并行 UI 审计（管理端 15 页 / 公开页与布局 / 共享组件与竞品对照），2026-08-29
> **状态**: 计划文档，待评审
> **原则**: 纯前端改动，零后端/API/协议变更；**保持现有 NodeGet 风格设计语言（Zinc 色板 + 描边卡片）不变**，只做一致性收敛与体验补齐，不推翻 8/28 已完成的 UI 重构成果

---

## 0. 审计结论摘要

UI 主体质量已达标（`card-soft` 覆盖广、双端组件同构、主题 FOUC 防护完整），但存在四类系统性问题：

| 问题类别 | 典型证据 | 影响面 |
|---------|---------|--------|
| **设计令牌落地不全** | `index.css:282` 定义 `.table-shell` 但全项目 0 处使用；CPU 图表用 `#3b82f6` 蓝而令牌 `--metric-cpu` 是青色；内存图表 `#8b5cf6` 紫 vs 令牌绿色 | 图表颜色与指标专色体系脱节，改主题改不动图表 |
| **加载/空/错误三态缺失** | 9 个管理页初始加载仅 spinner 无骨架；`ServerDetail.tsx:151` 历史加载失败仅 `console.error` 用户不可见；`AlertHistoryTimeline` 同病 | 数据未到时布局跳动、失败静默 |
| **页面间不一致** | 搜索框 `h-9` vs `h-10`；统计卡片字号 `text-xs`/`text-sm` 混用；`TrendCard`/数据库统计卡/流量图未用 `card-soft`；LogViewer 终端硬编码 `#1a1b26` 背景脱离主题体系 | 视觉细节毛糙 |
| **导航与状态语义** | `Layout.tsx:58` "系统状态"与"站点设置"共用同一齿轮图标；详情页 `document.title` 不更新；9 个管理表格缺 `overflow-x-auto` 小屏溢出；公告长文本可撑破布局 | 可用性与移动端体验 |

## 非目标（明确不做）

- ❌ 不更换设计语言（不做 Komari 风格切换、不引入组件库/新依赖）
- ❌ 不做全量表格分页——审计日志/告警历史已有分页，其余管理表数据量有限（个人规模），只补横向滚动
- ❌ 不做 i18n / 主题市场 / 地图联动 / SharePageView 独立化（列入"可选项"由评审决定）
- ❌ 不改任何 API、路由结构、状态管理逻辑

---

## 1. 任务总览

| 编号 | 任务 | 类型 | 预估 |
|------|------|------|------|
| W1.1 | Skeleton 骨架组件（card/table/chart 三形态） | 新组件 | 0.5h |
| W1.2 | EmptyState 统一空状态组件 | 新组件 | 0.5h |
| W1.3 | 图表与进度环颜色全面变量化 | 组件改造 | 1.5h |
| W1.4 | 图表三态 props（loading/error） | 组件改造 | 1h |
| W1.5 | `.table-shell` 落地为统一表格容器 | CSS+改造 | 0.5h |
| W2.1 | 9 个管理页表格补 `overflow-x-auto` + 最小宽度 | 批量修复 | 1h |
| W2.2 | 管理列表页初始加载换骨架屏 | 批量修复 | 1h |
| W2.3 | Dashboard 细节统一（输入高度/字号/指标专色/切换按钮） | 页面修复 | 1h |
| W2.4 | Settings/TrafficStats/SystemStatus 卡片与专色统一 | 页面修复 | 1h |
| W2.5 | LogViewer 主题令牌化 + 筛选胶囊统一 | 页面修复 | 1h |
| W3.1 | `Icon.system` 独立图标（修复齿轮重复） | 布局修复 | 0.5h |
| W3.2 | `usePageTitle` Hook（路由级 document.title） | 新 Hook | 0.5h |
| W3.3 | 移动端顶栏在线计数可见 | 布局修复 | 0.5h |
| W3.4 | 公告横幅长文本溢出保护 | 布局修复 | 0.5h |
| W4.1 | ServerDetail 五项修复（错误提示/TrendCard/进度条色/行 hover/移动端侧栏） | 页面修复 | 2h |
| W4.2 | ServerCard 离线态文字语义 | 组件修复 | 0.5h |
| W4.3 | AlertHistoryTimeline 失败提示 | 组件修复 | 0.5h |

可选项（默认不做，评审时决定）：
- W5.1 地图光点与卡片列表双向联动高亮（约 2h）
- W5.2 SharePageView 独立化——按 `SharePage.agent_ids` 过滤展示（约 2h，涉及数据流）

---

## 2. W1：基础组件统一（地基，先行）

### W1.1 Skeleton 组件

**新建**: `components/Skeleton.tsx`

三形态：`<Skeleton variant="card|table|chart" />`
- `card`: 统计卡骨架（标题条 + 大数值条）
- `table`: 表头行 + 5 行占位（复用 `.table-shell` 的行高节奏）
- `chart`: 固定高度色块（与各图表默认高度一致）

实现要点：`animate-pulse` + `bg-muted`，高度走各场景实际高度避免加载完成后跳动（CLS）。

### W1.2 EmptyState 组件

**新建**: `components/EmptyState.tsx`

```
<EmptyState icon? title description? action? />
```

统一替换 Dashboard（`829-848` 边框虚线风格）、各管理页（手写 SVG + 文案）、LatencyGrid/LatencyQualityBar 文字占位。图标默认用文档图标，允许传入自定义（如地图的定位图标）。

### W1.3 颜色全面变量化（核心）

现状：ECharts 组件硬编码色值，与 `--metric-*` 令牌脱节：

| 文件:行 | 现值 | 目标 |
|---------|------|------|
| `CpuChart.tsx:89` | `#3b82f6`（蓝） | `hsl(var(--metric-cpu))` 青 |
| `MemoryChart.tsx:89` | `#8b5cf6`（紫） | `hsl(var(--metric-mem))` 绿 |
| `CpuChart/MemoryChart:110,115` 阈值线 | `#f59e0b`/`#ef4444` | `hsl(var(--warning))`/`hsl(var(--destructive))` |
| `ResourceRing.tsx:26-39` | `#42b983`/`#f6ad55`/`#f56565` | `hsl(var(--success))`/`hsl(var(--warning))`/`hsl(var(--destructive))` |
| `ServerDetail.tsx:697` 磁盘进度条 | 同上三色 | 同上 |
| `CpuChart/MemoryChart:27,29,49,52,70` 轴线/文字 | `#444`/`#e5e7eb`/`#9ca3af`/`#6b7280`/`#1f2937` | `hsl(var(--border))`/`hsl(var(--muted-foreground))`/`hsl(var(--foreground))` |

实现方式：组件内通过 `getComputedStyle(document.documentElement).getPropertyValue()` 读取 HSL 变量（或在 `isDark` 分支中直接引用已有变量值），`PingChart.tsx:17-24` 的 `NETWORK_COLORS` 语义映射保留，底色调色板改为从 CSS 变量派生。

> 收益：主题切换时图表自动跟随；指标专色体系（CPU 青/内存绿/磁盘橙/网络粉/Ping 蓝）在卡片、统计、图表三处完全一致。

### W1.4 图表三态

`CpuChart`/`MemoryChart`/`PingChart`/`NetworkQualityChart` 增加 `loading?: boolean` / `error?: string` props：
- `loading` → 渲染 `<Skeleton variant="chart" />`
- `error` → 渲染内联错误条（destructive 描边 + 重试回调可选）
- 均为可选 props，不影响现有调用方

### W1.5 `.table-shell` 落地

`index.css:282` 已定义但零使用。两种选择（计划采用 A）：
- **A. 落地**：管理页表格外层统一 `card-soft overflow-hidden` + 内层 `table-shell`（含 `overflow-x-auto scrollbar-thin` + 表头样式），与 W2.1 一并完成
- B. 删除死代码

---

## 3. W2：管理页批量一致性修复

### W2.1 表格横向滚动（9 个页面）

为以下页面的表格容器补 `overflow-x-auto scrollbar-thin`，表格本体按列数设最小宽度：

`AgentManagement` / `AlertManagement`（规则+历史两表）/ `AuditLogs` / `NotifyChannels` / `PingTargets` / `ServiceMonitorManagement` / `SSLMonitorManagement` / `TagManagement` / `SharePageManagement`（补 `min-w`）

### W2.2 初始加载骨架

上述列表页的初始加载（`loading && data.length===0` 场景）从 spinner 换为 `<Skeleton variant="table" />`；刷新（已有数据）保留现有轻量表现，不闪烁。

### W2.3 Dashboard 细节

| 位置 | 修复 |
|------|------|
| `Dashboard.tsx:692` | 搜索框 `h-9` → `h-10`（与 `input-base` 规范对齐） |
| `Dashboard.tsx:581,627` | 统计卡描述统一 `text-xs text-muted-foreground` |
| `Dashboard.tsx:579-671` | CPU/内存/磁盘/网络统计数值加 `text-cpu`/`text-mem`/`text-disk`/`text-net` 专色 |
| `Dashboard.tsx:787-823` | 视图切换按钮内边距统一 `px-2.5`，与筛选胶囊节奏一致 |
| `Dashboard.tsx:829-848` | 空状态换 `EmptyState` 组件 |

### W2.4 Settings / TrafficStats / SystemStatus

- `Settings.tsx:334` 数据库统计卡 → `card-soft p-3`
- `Settings.tsx:185-188` 现有 pulse 骨架 → 复用 W1.1 Skeleton
- `TrafficStats.tsx` 月度柱状图容器 → `card-soft p-4` 包裹
- `SystemStatus.tsx:55,239,263` MetricCard 数值按指标加专色

### W2.5 LogViewer 主题化

- `LogViewer.tsx:263-264` 终端背景 `bg-[#1a1b26]` → `bg-card-elevated`（深浅主题各自适配合适底色；终端文字色同步令牌化）。注：日志查看器保持"终端感"，仅将硬编码色值换为可随主题的变量，浅色主题下呈现浅底深字终端
- `LogViewer.tsx:186-217` 日志级别筛选 → `filter-pill`/`filter-pill-active`/`filter-pill-inactive`

---

## 4. W3：布局与导航

### W3.1 独立系统状态图标

`Layout.tsx:58` "系统状态"当前复用 `Icon.settings`（与"站点设置"完全相同的齿轮）。新增 `Icon.system`（activity/仪表盘脉冲线图标），语义分离。

### W3.2 usePageTitle Hook

**新建**: `hooks/usePageTitle.ts`

```
usePageTitle('服务器详情')  // document.title = `${pageName} - ${siteTitle}`
```

应用位置：`ServerDetail`（服务器名）、`PublicServerDetail`、各管理页（页名）。站点标题变化时自动拼接（读 `useSiteSettingsStore`）。

### W3.3 移动端在线计数

`Layout.tsx:247-251` 在线计数 `hidden sm:flex` → 移动端以紧凑形式（仅 `n/m` 数字）显示在顶栏标题右侧。

### W3.4 公告溢出保护

`Layout.tsx:275` / `PublicLayout.tsx:81` 公告文本容器加 `break-words` + `max-h-[40vh] overflow-y-auto`（超长公告内部滚动，不撑破页面）。

---

## 5. W4：详情页与卡片体验

### W4.1 ServerDetail（五项）

1. **历史加载错误可见化**（`ServerDetail.tsx:151`）：新增 `historyError` state，图表区顶部渲染内联错误条（含"重试"按钮）
2. **TrendCard 卡片化**（`:1006`）：`rounded-md border bg-card/50 p-3` → `card-soft p-3`
3. **磁盘进度条颜色**（`:697`）：硬编码三色 → `--success`/`--warning`/`--destructive`（与 W1.3 同源）
4. **进程表行 hover**（`:943-946`）：补 `hover:bg-muted/50`，去掉隔行 `bg-secondary/10`（与全站表格节奏统一）
5. **移动端侧边栏**（`:504`）：`lg:` 以下将左侧硬件/系统/磁盘信息折叠为"概览"手风琴（`<details>` 或受控折叠），监控图表优先展示

### W4.2 ServerCard 离线态

`ServerCard.tsx:150-152,286-287`：状态行在离线时明确显示"离线 · 最后在线 xx 分钟前"（已有数据），替代仅 `opacity-80` + "---" 的弱表达；在线时维持现状。

### W4.3 AlertHistoryTimeline 失败提示

`AlertHistoryTimeline.tsx:27-30`：加载失败从静默 `histories=[]` 改为内联错误条 + 重试按钮（复用 W4.1 同款样式）。

---

## 6. 实施顺序

```
第 1 步  W1.1-W1.5 基础组件与颜色变量化          ← 纯新增+组件内改造，可独立验证
第 2 步  W2.1-W2.2 表格容器与骨架批量替换          ← 依赖 W1
第 3 步  W2.3-W2.5 页面细节统一                    ← 与第 2 步无依赖，可并行
第 4 步  W3.1-W3.4 布局导航                        ← 独立，随时可做
第 5 步  W4.1-W4.3 详情页与卡片                    ← W4.1.1 依赖 W1.4 图表三态
第 6 步  验证：tsc + vite build + 手动检查清单
```

## 7. 风险与回滚

| 风险 | 缓解 |
|------|------|
| ECharts 读 CSS 变量的时机（主题切换瞬间取旧值） | 变量读取放在 render 路径每次执行（非模块级缓存）；ECharts `isDark` prop 已存在，随主题重渲染即可拿到新值 |
| LogViewer 浅色终端观感突兀 | 浅色主题用 `card-elevated` 底 + 深字，先实现后视觉验收，不满意可单独回调该页深底（仅此一处例外并注释说明） |
| 批量替换骨架屏引入布局跳动 | Skeleton 高度取目标元素实际高度；刷新场景（已有数据）不换骨架 |
| 移动端侧边栏折叠丢失信息入口 | 默认展开第一屏可见，折叠交互用原生 `<details>` 无 JS 状态负担 |
| 回滚 | 全部为前端展示层改动，无数据/接口变更；单 commit 内聚，revert 即整体回滚 |

## 8. 验收标准

1. 全站无硬编码图表色值：`grep -r '#[0-9a-f]\{6\}' components/*.tsx` 仅剩 PingChart 语义映射表中允许的少量枚举色
2. 深浅主题切换后：图表/进度环/日志终端颜色全部跟随，无残留旧色
3. 9 个管理表格在 375px 宽度下可横向滚动，无页面级横向溢出
4. 断网/后端 500 场景：详情页图表区显示错误条与重试按钮，不再静默空白
5. 任意路由下浏览器标签页标题形如"页面名 - 站点名"
6. 弱网首屏：列表页呈现骨架而非空白或 spinner 跳动
7. `tsc --noEmit` + `vite build` 全绿；后端零改动（`git diff --stat server/internal` 为空）
