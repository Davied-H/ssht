# Group Management Iteration Design

## Context

`ssht` 当前的 group 体验存在两个层面的问题：

1. **实现偏离设计**：原 spec `2026-04-30-group-sidebar-tag-filter-design.md` 与 `README.md` 都明确要求左侧栏，但实际 `internal/app/view.go:151-183` 渲染的是顶部内联标签栏，且没有任何 sidebar 渲染逻辑。
2. **缺少 group 管理能力**：用户只能通过逐个编辑 host 间接维护 group，无法直接重命名、合并、排序、批量移动、创建空占位 group，表单也不会补全已有 group 名。

本次迭代解决以上两点。规模假设：4–8 个 group、不超过 50 个 host。

## Goal

恢复 spec 设计的左侧栏布局，并在侧栏内联完成 group 的增删改查、重命名、合并、排序、批量移动 host。表单 Group 字段升级为支持已有 group 补全。

## Non-Goals

- 不引入嵌套 group。
- 不允许 host 同时属于多个 group（仍是单值 `Group` 字段）。
- 不引入 command palette 或 vim-style ex 命令。
- 不实现 group 搜索（小规模下没必要）。
- 不动现有 host CRUD writer 的语义。

## 1. 屏幕布局与焦点

### 1.1 主屏三栏布局

```
┌─────────────────────────────────────────────────────────────┐
│ ssht  Hosts 24  Matched 8  ★ 5  ⏱ 7  ✓ 2  ⚠ 1              │
├─ Groups ───┬─ Hosts ───────────────┬─ Preview ──────────────┤
│ all     24 │ > prod-api-01         │ HostName 192.0.2.12     │
│ ▸ prod   8 │   prod-api-02         │ User     deploy        │
│   dev    5 │   prod-db-01          │ Port     22            │
│   stage  3 │   ...                 │ Source   ~/.ssh/config │
│   ungr.  4 │                       │                        │
│            │ / filter…             │                        │
├────────────┴───────────────────────┴────────────────────────┤
│ Tab: focus · Enter: open · Space: mark · ?: help            │
└─────────────────────────────────────────────────────────────┘
```

- 顶部 dashboard 行只放计数（Hosts / Matched / Favorites / Recent / Selected / Warnings），**移除原顶部 group 标签栏**。
- 左 sidebar 永久显示，宽度按最长 group 名 + 计数自适应（14–18 字符之间）。
- `▸` 标记当前选中 group；高亮 border 标记当前焦点。

### 1.2 焦点模型

新增 `Model.focus` 字段，取值 `focusList` / `focusSidebar`。`Tab` / `Shift-Tab` 切换。

### 1.3 窄终端退化

- 终端宽度 `< twoColumnMinWidth`：sidebar 不渲染，回退到当前单列模式；`[ ]` 仍可切 group。
- 宽度足够 sidebar + list 但不够 preview：优先牺牲 preview，保留 sidebar + list。

### 1.4 涉及文件

- `internal/app/view.go`：重写 `topStatusLine`、`splitColumns`，新增 `sidebarColumn`。
- `internal/app/model.go`：新增 `focus` 字段、`Tab` 键处理；保留 `[ ]` `←/→` 全局生效。

## 2. 数据模型与持久化

### 2.1 不变部分

```go
// internal/sshconfig/parser.go:24-38
type HostEntry struct {
    Group  string   `json:"group,omitempty"`
    Tags   []string `json:"tags,omitempty"`
    // ...
}
```

`HostEntry.Group` 仍是单值字符串。SSH config 是真相之源。

### 2.2 state.json 新增字段

```go
type State struct {
    // 现有字段...
    GroupOrder  []string `json:"groupOrder,omitempty"`
    EmptyGroups []string `json:"emptyGroups,omitempty"`
}
```

- `GroupOrder`：用户手动排序的 group 名序列。
- `EmptyGroups`：占位空 group 的名字列表（用户先建分类、后填 host 的需要）。

### 2.3 显示规则（重写 `groupItems()`）

伪代码：

```
S_known   = (groups present in ssh config) ∪ state.EmptyGroups
ordered   = [g for g in state.GroupOrder if g in S_known]   # 过滤已不存在的过期项
unordered = sorted([g for g in S_known if g not in state.GroupOrder])
display   = ["all"] + ordered + unordered + (["ungrouped"] if any host without group)
```

- `all` 固定置首；`ungrouped` 固定置尾，且仅在存在未分组 host 时出现。
- `state.EmptyGroups` 中的项以计数 `0` 显示。
- `state.GroupOrder` 永远是用户主动排序意图的体现，不会被自动膨胀。

### 2.4 状态同步表

| 用户动作 | state.json 改动 | ssh config 改动 |
|---|---|---|
| `J/K` 调顺序 | `GroupOrder` 重排（如目标 group 还不在 GroupOrder 中，按当前显示位置注入） | 无 |
| `a` 创建空 group | `EmptyGroups` 追加 | 无 |
| 把 host 进空 group | `EmptyGroups` 移除该项 | 改 host 注释 |
| `r` 重命名 group | `GroupOrder` / `EmptyGroups` 同步改名 | 批量改所有相关 host 注释 |
| `m` 合并 group | 删除被合并项 | 把源 group 注释全改成目标 group |
| `d` 删除 group | `GroupOrder` / `EmptyGroups` 移除 | 把该 group 下所有 host 注释中的 `group=` 拿掉 |
| 删除 host 后 group 为空 | 不动（保留为空 group 占位） | 由现有 writer 处理 |

### 2.5 不变量

- `ungrouped` / `all` 是保留名，不能 rename / delete / 出现在 `GroupOrder` / `EmptyGroups`。
- 任何 group 名出现在 `EmptyGroups` 时，不允许同时出现在 ssh config 注释里。
- 模型层在每次 reload 后清理 `EmptyGroups` 中已被引用的项。

## 3. 键位与交互

### 3.1 焦点切换

| 键 | 动作 |
|---|---|
| `Tab` / `Shift-Tab` | `focusList` ↔ `focusSidebar` |
| `[` `]` `←` `→` | 全局生效，切换当前选中 group |

### 3.2 Sidebar 焦点下的键位

| 键 | 动作 |
|---|---|
| `↑` `↓` `j` `k` | 在 group 列表内移动 |
| `Enter` | 选定并把焦点切回 list |
| `a` | 内联输入框创建空 group |
| `r` | 内联输入框重命名当前 group（预填当前名） |
| `m` | 弹下拉选合并目标 → 进 confirm |
| `d` | 进 confirm（提示 N 个 host 会进入 ungrouped） |
| `M` | 把 `Space` 标记的 host 移到当前 group → 进 confirm |
| `J` `K` | 当前 group 在 GroupOrder 中下移 / 上移 |
| `Esc` | 取消内联输入；否则切回 list 焦点 |

### 3.3 表单 Group 字段补全

- 在 `modeForm`、Group 字段聚焦时按 `Tab` 弹下拉。
- 下拉来源：`groupItems()` 去掉 `all` / `ungrouped` + 行末追加 `+ new...`。
- 方向键选择，`Enter` 确认；继续输入则保留为新 group 名。

### 3.4 模式扩展

新增 `modeGroupInline`（轻量级，只用于 `a` `r`）。`m` `d` `M` 复用现有 `modeConfirm`。

### 3.5 Confirm 面板示例

```
Confirm merge:
  Source group : staging  (3 hosts)
  Target group : dev
  Will rewrite : ~/.ssh/config (2 entries)
                 ~/.ssh/config.d/work (1 entry)
  Backups      : .ssht.20260504-101530.bak

s confirm  Esc cancel
```

### 3.6 帮助页 (`?`) 增补

新增「Sidebar」段，列出 §3.2 所有键。

## 4. 写操作（SSH config 批量改注释）

### 4.1 新文件 `internal/sshconfig/groupwriter.go`

```go
func RenameGroup(sources []string, oldName, newName string) (BatchResult, error)
func MergeGroup(sources []string, fromName, toName string) (BatchResult, error)
func DeleteGroup(sources []string, name string) (BatchResult, error)
func MoveHostsToGroup(entries []HostEntry, targetName string) (BatchResult, error)

type BatchResult struct {
    FilesChanged []string
    HostsChanged int
    Backups      []string
}
```

### 4.2 实现策略

1. **粒度**：按 `HostEntry.SourceFile` + `SourceLine` 反查注释行，逐行替换。
2. **多文件 best-effort 原子性**：
   - 第 1 阶段：每个目标文件计算新内容 → 写到 `<file>.ssht.tmp`。
   - 第 2 阶段：每个目标文件做时间戳备份 + `os.Rename(tmp → orig)`。
   - 中途失败：已 rename 的文件不回滚（同时间戳的 `.bak` 还在），未 rename 的临时文件清理；返回错误列出已改 / 未改文件。
   - 不实现真正事务回滚（隔离区复制）—— 当前规模 YAGNI。
3. **复用现有 backup 行为**：复用现有 writer 的 backup helper。
4. **写完后强制 reload**：`m.reloadConfig()`。

### 4.3 注释格式抽出

把 `parseMetadataComment` 的逆向写法抽到 `renderMetadataComment`，由现有 writer 与 `groupwriter` 共享。

### 4.4 副作用边界

- 只动 `# ssht: group=` 部分，不动 tags、Host 字段、其他注释。
- host 没有注释时，rename 不影响该 host。
- 注释里删 group 后只剩空载荷时，删除整行注释。

### 4.5 错误处理矩阵

| 情景 | 行为 |
|---|---|
| 新名为空 / 含 ` ` `,` `#` `=` | 阻止，status 行红字提示 |
| 新名 = `all` / `ungrouped` | 阻止，提示是保留名 |
| Rename 时新名已存在 | 阻止，提示「请用 merge」 |
| Merge 时目标 group 不存在 | 阻止，提示「目标必须是已有 group」 |
| 文件写权限失败 | 已写文件不回滚，status 列出失败文件 |
| Source file 缺失 | 跳过该 host，写完后 status 提示 |

## 5. 测试

### 5.1 `internal/sshconfig/groupwriter_test.go`（新文件）

- `TestRenameGroup_singleFile`
- `TestRenameGroup_multiFile`
- `TestMergeGroup_collisionsResolved`
- `TestDeleteGroup_stripsCommentLine`：注释只剩 `group=` 时删整行；与 tags 共存时只删 group= 部分
- `TestRenameGroup_reservedNameRejected`
- `TestRenameGroup_collidingNameRejected`
- `TestRenameGroup_partialFailureRollsForward`

### 5.2 `internal/state/state_test.go`（追加）

- `TestState_GroupOrderRoundTrip`
- `TestState_EmptyGroupsRoundTrip`
- `TestState_GroupOrderMigratesFromMissingField`

### 5.3 `internal/app/model_test.go`（追加）

- `TestGroupItems_orderingFromState`
- `TestGroupItems_includesEmptyGroups`
- `TestSidebarFocus_keysScoped`
- `TestRenameInline_validation`
- `TestMoveMarkedToGroup`
- `TestFormGroupCompletion`

### 5.4 `internal/app/view_test.go`（新文件或追加）

- `TestSidebarRender_smallTerminal`
- `TestSidebarRender_widthAdaptsToLongestName`
- `TestSidebarRender_focusIndicator`

### 5.5 手工验证清单

1. testdata 准备 3 group / 6 host，启动 `ssht`，sidebar 正确显示。
2. Tab 进 sidebar，`r` 改 prod → production，列表注释已改、备份生成。
3. `m` staging → dev，staging 消失、原 host 全在 dev 下。
4. `Space` 标 2 个 host，到 sidebar 选 staging，按 `M`，移动成功。
5. `J/K` 改顺序，重启后顺序保留。
6. `--no-include` 与多文件 Include 场景下 rename 跨文件正确。

### 5.6 不覆盖范围

- 不测 ANSI 颜色（视觉自检）。
- 不测 bubble tea 内部消息流。
- 不测 ssh 连接本身。

## 6. 关键文件清单

| 路径 | 改动类型 | 说明 |
|---|---|---|
| `internal/app/view.go` | 重写 | 删 `topStatusLine` 内联 group；新增 `sidebarColumn`；改 `splitColumns` |
| `internal/app/model.go` | 修改 | 新增 `focus` / `Tab` 键 / sidebar 操作分发；保留 `[ ]` 切 group |
| `internal/app/model.go` | 修改 | 表单 Group 字段下拉补全（与 `focus` 改动同文件） |
| `internal/state/state.go` | 修改 | 新增 `GroupOrder` / `EmptyGroups` 字段 |
| `internal/sshconfig/parser.go` | 微调 | 抽出 `renderMetadataComment` |
| `internal/sshconfig/groupwriter.go` | **新建** | RenameGroup / MergeGroup / DeleteGroup / MoveHostsToGroup |
| `README.md` | 更新 | 键位表新增 sidebar 段；左侧栏说明已经写了，确认与实现一致 |

## 7. 实施顺序建议

1. 第 2 节数据层（state.go 字段、`groupItems()` 重写、模型层不变量）+ 单元测试。
2. 第 4 节 `groupwriter.go` + 单元测试。
3. 第 1 节布局（view.go 三栏 + 窄屏退化）+ view 测试。
4. 第 3 节键位与 modeGroupInline；表单补全。
5. 帮助页与 README 更新。
6. 手工验证清单走一遍。
