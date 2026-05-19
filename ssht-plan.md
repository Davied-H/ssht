# 基于 ssh config 的 tmux SSH TUI 工具实施计划

## Requirements Summary

目标是实现一个本地 TUI 工具，从 `~/.ssh/config` 及其 `Include` 文件读取可连接的 `Host` 条目，提供键盘驱动的筛选、预览、连接、复用和管理能力。用户在 TUI 中选择 Host 后，工具通过 `tmux` 创建或复用对应连接，而不是直接在当前进程里阻塞执行 `ssh`。

默认产品形态：

- 单一可执行命令，例如 `ssht` 或 `sshui`。
- 启动后进入全屏 TUI，展示 SSH Host 列表。
- 支持模糊搜索、分组/标签展示、配置预览、连接状态提示。
- 回车连接：创建或复用一个稳定命名的 `tmux` session/window/pane，并在其中执行 `ssh <host>`。
- 支持 attach 到已存在连接。
- 支持不修改用户 SSH 配置，只读取配置并调用系统 `ssh`/`tmux`。

非目标：

- 不实现 SSH 协议客户端。
- 不保存 SSH 密码、私钥或凭据。
- 不替代 OpenSSH 的配置解析语义，只做足够可靠的 Host 发现和展示，连接时仍交给 `ssh <host>`。

## Recommended Stack

如果没有现有代码约束，建议使用 Go：

- TUI：`github.com/charmbracelet/bubbletea`
- UI 组件：`github.com/charmbracelet/bubbles`
- 样式：`github.com/charmbracelet/lipgloss`
- 测试：Go 标准 `testing`，必要时用 fixture 文件测试解析器

理由：

- Go 生成单一二进制，适合本地 CLI/TUI 工具分发。
- Bubble Tea 对全屏 TUI、键盘事件、状态机和测试较成熟。
- 调用 `ssh`/`tmux` 这类系统命令简单直接。

备选：

- Rust + `ratatui`：类型安全和性能强，但实现成本更高。
- Node.js + `ink`：开发快，但分发和系统依赖较重。

## Proposed File Layout

实现阶段建议创建以下文件：

- `cmd/ssht/main.go`：程序入口、参数解析、依赖装配。
- `internal/sshconfig/parser.go`：解析 `~/.ssh/config`、`Include`、Host block、基础字段。
- `internal/sshconfig/parser_test.go`：SSH config fixture 测试。
- `internal/tmux/tmux.go`：封装 `tmux` 命令、session/window 命名、存在性检测、attach/switch。
- `internal/tmux/tmux_test.go`：命令构造和命名策略测试。
- `internal/app/model.go`：Bubble Tea model、消息、状态转移。
- `internal/app/view.go`：列表、预览、状态栏、错误展示。
- `internal/app/keymap.go`：快捷键定义。
- `internal/app/model_test.go`：TUI 状态机测试。
- `go.mod` / `go.sum`：Go 模块定义。
- `README.md`：安装、使用、快捷键、限制说明。

## Architecture

### 1. SSH Config Discovery

输入：

- 默认路径：`~/.ssh/config`
- 可选参数：`--config /path/to/config`

输出结构建议：

```go
type HostEntry struct {
    Alias       string
    HostName    string
    User        string
    Port        string
    IdentityFile string
    ProxyJump   string
    Tags        []string
    SourceFile  string
    SourceLine  int
    RawBlock    string
}
```

解析规则：

- 读取 `Host` block，排除包含 `*`、`?`、`!` 的通配模式作为可直接连接项。
- 一个 `Host` 行含多个别名时，为每个非通配别名生成 `HostEntry`。
- 支持常见字段：`HostName`、`User`、`Port`、`IdentityFile`、`ProxyJump`、`ProxyCommand`。
- 支持 `Include`，处理相对路径、`~`、glob。
- 检测 Include 循环，避免无限递归。
- 保留源文件和行号，用于 TUI 预览和调试。

连接命令不需要重建完整 SSH 参数，始终优先执行：

```bash
ssh <HostEntry.Alias>
```

这样可以让 OpenSSH 自己处理复杂配置、Match、Include、ProxyCommand、ControlMaster 等语义。

### 2. tmux Orchestration

核心策略：

- 工具不直接持有 SSH 子进程。
- 每个 Host 对应一个稳定 tmux session 名。
- session 存在时优先 attach/switch，不重复创建连接。

命名规则：

```text
ssht:<sanitized-host-alias>
```

例如：

- `prod-api` -> `ssht:prod-api`
- `root@192.0.2.1` -> `ssht:root-192-0-2-1`

命令策略：

- 检查 tmux 是否存在：`tmux -V`
- 检查 session：`tmux has-session -t <session>`
- 创建 session：`tmux new-session -d -s <session> 'ssh <alias>'`
- attach 或切换：
  - 当前在 tmux 内：`tmux switch-client -t <session>`
  - 不在 tmux 内：`tmux attach-session -t <session>`

需要注意：

- `tmux` session 名必须严格转义/参数化，避免 shell 注入。
- 启动 `ssh` 时不要拼接 shell 字符串；优先使用 `exec.Command("tmux", "new-session", "-d", "-s", session, "ssh", alias)`。如果 tmux 对命令参数行为不满足，再封装最小 shell 并对 alias 做严格 allowlist。
- attach 会接管终端，TUI 需要先退出 Bubble Tea alternate screen。

### 3. TUI Interaction

主界面布局：

```text
┌──────────────────────────────────────────────┐
│ Search: prod                                 │
├───────────────────────┬──────────────────────┤
│ > prod-api            │ HostName: 192.0.2.12   │
│   prod-db             │ User: deploy          │
│   staging-api         │ Port: 22              │
│   jump-box            │ ProxyJump: bastion    │
│                       │ Source: ~/.ssh/config │
├───────────────────────┴──────────────────────┤
│ Enter connect | a attach | r reload | q quit  │
└──────────────────────────────────────────────┘
```

快捷键：

- `/` 或直接输入：筛选 Host。
- `Enter`：连接或切换到所选 Host 的 tmux session。
- `a`：attach/switch 到已有 session；如果不存在则提示。
- `n`：强制创建新 session，session 名追加递增后缀。
- `r`：重新加载 SSH config。
- `c`：复制 `ssh <alias>` 命令到剪贴板，作为可选能力。
- `?`：显示帮助。
- `q` / `Esc`：退出。

状态展示：

- 已存在 tmux session 的 Host 显示 `active` 标记。
- 配置解析错误显示在底部状态栏，但不阻止展示已解析 Host。
- 连接动作失败时返回 TUI 并展示错误。

### 4. CLI Flags

建议支持：

```bash
ssht --config ~/.ssh/config
ssht --tmux-prefix ssht
ssht --no-include
ssht --debug
ssht --print-hosts
```

其中 `--print-hosts` 用于自动化测试和脚本集成，输出 JSON 或表格。

## Implementation Steps

1. 初始化 Go 项目与骨架
   - 创建 `go.mod`、`cmd/ssht/main.go`、`internal/...` 目录。
   - 入口先支持 `--print-hosts`，便于先验证解析器。

2. 实现 SSH config 解析器
   - 解析 `Host` block 和常见字段。
   - 实现 `Include`、glob、循环保护。
   - 添加 fixture 覆盖多 Host、通配 Host、Include、注释、缩进、字段覆盖。

3. 实现 tmux 封装层
   - 检查 tmux 可用性。
   - 实现 session 命名、存在性检测、创建、attach/switch。
   - 测试命令构造，不在单元测试里真实创建 tmux。

4. 实现 TUI model
   - 加载 Host 列表。
   - 支持筛选、上下移动、帮助弹层、reload。
   - 将 connect/attach 动作建模为 Bubble Tea command。

5. 接入 tmux 连接流程
   - `Enter` 时退出 alternate screen，创建/复用 tmux session。
   - 根据是否处于 tmux 内选择 `switch-client` 或 `attach-session`。
   - 失败时回到 TUI 并展示错误。

6. 打磨 README 与安装方式
   - 文档包含安装、快捷键、tmux 行为、SSH config 支持范围、常见问题。
   - 提供 `go install ./cmd/ssht` 或 release 构建说明。

## Acceptance Criteria

- 给定包含 5 个普通 `Host`、2 个通配 `Host`、1 个 `Include` 文件的 fixture，`--print-hosts` 只输出可直接连接的普通 Host，并保留来源文件和行号。
- 对 `Host prod-api prod-api.internal`，工具生成两个可筛选条目，连接命令分别为 `ssh prod-api` 和 `ssh prod-api.internal`。
- 对 `Host *`、`Host *.internal`、`Host !blocked`，工具不把这些模式作为可直接连接项展示。
- 当 `tmux has-session -t ssht:prod-api` 成功时，按 `Enter` 不创建新 session，而是 attach/switch 到现有 session。
- 当 session 不存在时，按 `Enter` 创建 `ssht:prod-api`，并在其中执行 `ssh prod-api`。
- 在 tmux 内运行工具时，连接动作使用 `switch-client`；不在 tmux 内运行时，使用 `attach-session`。
- `r` 重新加载配置后，新加入的 Host 能出现在列表中，已删除的 Host 从列表消失。
- `tmux` 不存在时，TUI 启动后展示明确错误，不 panic。
- 配置文件不存在时，TUI 显示空状态和路径提示，不 panic。
- 单元测试覆盖解析器、session 命名、tmux 命令构造、TUI 筛选状态转移。

## Risks and Mitigations

- 风险：OpenSSH config 语义很复杂，完整复刻成本高。
  - 缓解：工具只负责发现和展示 Host，连接始终调用 `ssh <alias>`，让 OpenSSH 自己解析最终配置。

- 风险：tmux/ssh 命令拼接导致 shell 注入。
  - 缓解：优先使用参数数组调用命令；session 名使用严格 sanitize；alias 不进入 shell 字符串。

- 风险：`Include` 循环或 glob 范围过大导致卡顿。
  - 缓解：记录 visited 文件，限制递归深度，解析过程设置合理超时或最大文件数。

- 风险：attach tmux 会打断 TUI alternate screen。
  - 缓解：在 Bubble Tea command 中显式退出程序或释放终端后再 attach。

- 风险：用户期望一个 Host 多开多个连接。
  - 缓解：默认复用，提供 `n` 强制创建带后缀的新 session。

## Verification Steps

1. 运行单元测试：

```bash
go test ./...
```

2. 用 fixture 验证 Host 输出：

```bash
ssht --config ./testdata/ssh_config --print-hosts
```

3. 在没有 tmux 的环境模拟错误路径，确认 TUI 不 panic。

4. 在真实 tmux 环境手动验证：

```bash
tmux kill-session -t ssht:prod-api 2>/dev/null || true
ssht --config ./testdata/ssh_config
tmux has-session -t ssht:prod-api
```

5. 在 tmux 内外分别验证 `switch-client` 与 `attach-session` 行为。

## Milestone Plan

- M1：解析器 + `--print-hosts`，可列出 SSH Host。
- M2：tmux 封装 + 测试，能创建/复用 session。
- M3：TUI 列表、搜索、预览、reload。
- M4：TUI 与 tmux 连接集成。
- M5：异常处理、文档、安装说明、手动验证。

## Open Decisions

- 命令名：默认建议 `ssht`，短且表达 SSH + tmux。
- 是否支持最近连接排序：建议第一版用字母排序，后续再加本地状态文件。
- 是否支持分组：第一版可从 Host 前缀推断简单分组，例如 `prod-*`，不引入配置文件。
- 是否支持保存用户偏好：第一版不保存，避免状态复杂化。
