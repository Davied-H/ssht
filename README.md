# ssht

![ssht terminal UI preview](assets/ssht-hero.png)

`ssht` 是一个本地 TUI 工具，用来从 OpenSSH config 中浏览并打开 SSH
主机。

它会读取 `~/.ssh/config` 以及被 `Include` 引入的配置文件，列出具体可连接的
`Host` 别名，并通过新建终端窗口或新 tab 执行 `ssh <alias>` 来连接主机。它也支持新增、
编辑、删除 `Host` 配置块；写入前会显示确认步骤，并在写入前创建带时间戳的备份。
`ssht` 不实现 SSH 协议，也不保存任何凭据。

## 安装

推荐从 GitHub Release 一键安装，不需要本机安装 Go：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/install-release.sh | sh
```

默认安装到 `~/.local/bin/ssht`。指定安装目录：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/install-release.sh | INSTALL_DIR=/usr/local/bin sh
```

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/install-release.sh | SSHT_VERSION=v0.1.0 sh
```

如果你已经 clone 了仓库，也可以直接运行：

```bash
sh scripts/install-release.sh
```

仓库仍为私有时，`raw.githubusercontent.com` 的一键命令需要你自己带 GitHub 鉴权；
更直接的方式是先 clone 仓库，然后运行上面的本地脚本。

源码构建安装需要 Go：

```bash
./install.sh
```

源码安装脚本同样默认安装到 `~/.local/bin`。如果要指定其他目录：

```bash
INSTALL_DIR=/usr/local/bin ./install.sh
```

## Codex / Claude Code 一键配置

本仓库带有给 Codex 和 Claude Code 看的项目说明，以及三个配套 Codex skills：

- `ssht-config-auditor`：审查 SSH config 示例、`# ssht:` 元数据和公开前敏感信息。
- `ssht-release-packager`：处理 release、tag、安装脚本和跨平台包验证。
- `ssht-raycast-helper`：维护 Raycast extension、偏好项和连接动作。

一键安装这些 agent 配置和 skills：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/configure-agents.sh | sh
```

在本地仓库内，Codex 会读取 `AGENTS.md`，Claude Code 会读取 `CLAUDE.md`。如果你想让
Codex 在其他会话中也能使用这些 skills，可以在 clone 后运行：

```bash
sh scripts/configure-agents.sh
```

## GitHub 发布

仓库内置 GitHub Actions：

- `CI`：在 push、pull request 和手动触发时运行 Go 测试，并构建 Raycast extension。
- `Release`：推送 `v*` tag 或手动触发时，先运行测试，再构建 macOS、Linux、Windows
  的 `ssht` 发布包，生成 `checksums.txt`，并自动上传到 GitHub Release。

正式发布：

```bash
git tag v1.0.0
git push origin v1.0.0
```

也可以在 GitHub Actions 页面手动运行 `Release` workflow，填写版本号，例如
`v1.0.0`。发布产物会自动附加到对应 GitHub Release。

## 使用

```bash
ssht
ssht --config ~/.ssh/config
ssht --no-include
ssht --debug
ssht --print-hosts
ssht --terminal terminal
ssht --open-mode tab
ssht --monitor
ssht --connect prod-api-01
```

`--print-hosts` 会把发现的主机以 JSON 输出，方便脚本和测试夹具使用。
`--connect <alias>` 会跳过 TUI，直接按当前终端设置打开指定 `Host` 别名，并记录到本地
最近连接状态；这个入口主要供 Raycast 等外部启动器调用。
`--no-include` 会禁用 `Include` 递归解析。`--debug` 会把解析 warning 输出到
stderr，同时仍然展示已经成功解析的主机。`--terminal` 用来选择终端后端，支持
`auto`、`iterm`、`terminal`、`wezterm`、`kitty`、`alacritty` 和 `ghostty`。
默认值是 `auto`，也可以通过 `SSHT_TERMINAL` 环境变量设置。
`--open-mode` 用来选择打开方式，支持 `auto`、`window` 和 `tab`。默认值是 `auto`，
也可以通过 `SSHT_OPEN_MODE` 环境变量设置。
`--monitor` 在启动时直接打开右侧的 SSH 监控面板（Health + Top CPU）；
默认不显示，进入 TUI 后可以随时按大写 `M` 切换显隐。

## 快捷键

- 直接输入文字：筛选主机
- `/`：清空当前筛选并回到完整列表
- `tag:<name>`：按标签筛选主机
- `fav:`：只显示收藏主机
- `recent:`：只显示曾经打开过的主机
- `[` / `]` 或 Left / Right：切换分组
- `PgUp` / `PgDn` / `Home` / `End`：快速移动主机列表光标
- `Tab`：在主机列表与左侧分组侧栏之间切换焦点
- `Enter`：按配置的终端打开方式连接选中的主机
- `Space`：标记或取消标记主机，用于跨筛选/跨分组批量打开
- `f`：收藏或取消收藏选中的主机
- `e`：编辑选中的主机
- `A`：新增主机
- `d`：删除选中的主机
- `Ctrl+S`：在新增/编辑表单中进入保存确认
- `s`：确认待执行的写入操作
- `g`：把当前主机或已标记主机移动到分组（可选择已有分组或输入新分组名）
- `r`：重新加载 SSH config
- `R`：立即刷新当前主机的 monitor 快照（会自动打开 monitor 面板）
- `W`：查看 SSH config 解析 warning 列表
- `?`：帮助
- `q` / `Esc`：退出

侧栏聚焦（按 `Tab` 切到分组侧栏）后可用以下键管理 group：

- `j` / `k`：在分组列表上下移动
- `a`：新建一个空 group 占位（仅写入本地 state，不动 SSH config）
- `r`：重命名当前 group（批量重写所有相关 host 的 `# ssht: group=` 注释）
- `m`：把当前 group 合并进另一个 group
- `d`：删除当前 group（成员主机自动落入 `ungrouped`）
- `M`：把已用 `Space` 标记的主机批量移动到当前 group（高级路径；主机列表里按 `g` 更快）
- `J` / `K`：在保存的顺序中把当前 group 下移 / 上移
- `Esc`：把焦点切回主机列表

新增 / 编辑表单中聚焦在 `Group` 字段时按 `Tab` 可在已有 group 名间循环，避免拼错产生新分组。

当使用 `Space` 标记了一个或多个主机后，按 `Enter` 会打开所有已标记主机，即使其中一部分当前因为筛选或分组切换不可见。
成功连接会记录到本地状态中，用于收藏、最近使用、连接次数以及默认排序。

新增 / 编辑表单支持字段内光标移动：Left / Right 移动光标，`Ctrl+A` / `Ctrl+E`
跳到当前字段开头 / 末尾，`Ctrl+U` 清空当前字段。

顶部 dashboard 会展示主机总数、当前筛选命中数、本地连接状态计数、已选择主机数
和解析 warning 数：

```text
Hosts 24 | Matched 8 | Favorites 5 | Recent 7 | Selected 2 | Warnings 1
```

左侧边栏会按 `ssht` 元数据注释对主机分组。没有分组的主机会显示在 `ungrouped`
下面。

```sshconfig
# ssht: group=prod tags=api,critical
Host prod-api-01
    HostName 192.0.2.12
    User deploy
```

`group` 是主浏览分组。`tags` 是独立标签，用于搜索筛选，例如 `tag:api` 或
`tag:api tag:critical`。

`fav:`、`recent:`、`tag:<name>` 和普通搜索词可以组合使用。例如，
`fav: tag:api prod` 会显示收藏的、带有 `api` 标签、并且匹配 `prod` 的主机。

## 本地状态

`ssht` 会把收藏、最近连接时间和连接次数保存在自己的状态文件中。它不会保存
HostName、User、IdentityFile、密码、私钥或其他 SSH 凭据。

状态文件路径如下：

- 设置了 `XDG_STATE_HOME` 时：`$XDG_STATE_HOME/ssht/state.json`
- macOS 默认：`~/Library/Application Support/ssht/state.json`
- 其他系统默认：`~/.local/state/ssht/state.json`

这些状态是用户本地偏好，不会写入 SSH config 文件。

## 终端行为

按下 `Enter` 时，`ssht` 会请求选中的终端按配置的打开方式执行：

```bash
ssh <alias>
```

如果 `Host` 前紧挨着 `# sshpass 密码: ...` 注释，则会执行：

```bash
sshpass -p <password> ssh <alias>
```

选中的别名仍然由 OpenSSH 解析，因此所有连接细节都来自你的 SSH config。

默认情况下，`ssht` 会按以下顺序自动选择第一个可用终端：iTerm2、Terminal.app、
WezTerm、kitty、Alacritty、Ghostty。要强制指定后端，可以使用
`--terminal <name>`，或设置 `SSHT_TERMINAL`。

默认打开方式是自动模式：

```bash
ssht --open-mode auto
```

在 iTerm2 会话内运行时，`auto` 会在当前窗口右侧竖向分屏并执行连接；不在
iTerm2 会话内时，`auto` 会按新 tab 行为打开连接。

要强制使用新窗口：

```bash
ssht --open-mode window
```

要改为新 tab：

```bash
ssht --open-mode tab
SSHT_OPEN_MODE=tab ssht
```

`tab` 和 `window` 是严格语义：显式指定后不会因为当前在 iTerm2 内而自动分屏。
如果指定的终端后端不支持新 tab，连接会失败并显示错误，不会自动退回新窗口。
`auto` 模式会跳过不支持当前打开方式的后端。当前新 tab 支持 iTerm2、
Terminal.app、WezTerm、kitty 和 Ghostty；Alacritty 只支持 `window`。
kitty 的新 tab 依赖 kitty remote control，如果本机未启用，对应命令会失败并由
TUI 展示错误。若支持 tab 的终端当前没有窗口，`ssht` 会创建首个承载连接的窗口，
因为 tab 必须隶属于窗口。

## SSH config 支持范围

解析器会发现直接可连接的 `Host` 别名，并提取常用字段用于展示：
`HostName`、`User`、`Port`、`IdentityFile`、`ProxyJump` 和 `ProxyCommand`。
`Include` 支持相对路径、`~`、环境变量和 glob。

`ssht` 还会读取紧挨在 `Host` 配置块之前的可选元数据注释。这些注释会被
OpenSSH 忽略；当新增或编辑主机并设置 group 或 tags 时，`ssht` 会重写这些注释。
如果某个 Host 只是给 Git、rsync 等工具使用，不想在主机列表中展示，可以加
`hidden=true`：

```sshconfig
# ssht: hidden=true
Host github.com
    HostName github.com
    User git
```

兼容 `sshm` 风格的 sshpass 注释：

```sshconfig
# sshpass 密码: example-password
Host password-host
    HostName 192.0.2.12
    User root
```

该密码只在内存中用于连接命令，不会出现在 `--print-hosts` 的 JSON 输出中。编辑该
Host 时，`ssht` 会保留已有 sshpass 注释，但不会在表单中展示密码字段。

通配或否定模式不会作为可直接连接条目展示，例如 `Host *`、`Host *.internal`
和 `Host !blocked`。连接时仍然执行 `ssh <alias>`，完整配置语义由 OpenSSH
负责处理。

## FAQ 和限制

### ssht 会解析所有 OpenSSH 特性吗？

不会。它只发现具体的 `Host` 别名并展示常用字段，不会尝试完整求值 `Match`、
token 展开、条件 Include 或最终生效的 SSH 选项。实际连接始终委托给 OpenSSH：
`ssh <alias>`。

### 首选终端不存在时会怎样？

TUI 仍然会启动，并显示清晰的状态 warning。连接会失败，直到系统中存在支持的终端，
或你通过 `--terminal` 选择其他可用后端。

### ssht 会修改我的 SSH config 吗？

只有当你使用新增、编辑或删除功能时才会修改。写入前，`ssht` 会展示目标文件并要求
确认。它会写入选中主机所在的源文件，包括被 Include 引入的配置文件，并在同目录下
创建带时间戳的 `.ssht.<time>.bak` 备份。

### 能否多次打开同一个主机连接？

可以。再次按 `Enter` 会按当前打开方式为同一个主机再打开一个新的连接。

## Raycast 扩展

仓库内置了一个配套 Raycast extension，位于 `raycast/`。它会调用：

- `ssht --print-hosts`：读取并搜索 SSH config 主机
- `ssht --connect <alias>`：从 Raycast 选中主机后打开 SSH 连接

首次使用：

```bash
cd raycast
npm install
npm run dev
```

如果 Raycast 找不到 `ssht`，先在项目根目录运行 `./install.sh`，然后到 Raycast
extension preferences 里把 `ssht Path` 设置为安装脚本输出的绝对路径，例如
`/Users/<you>/.local/bin/ssht`。

可配置项：

- `ssht Path`：`ssht` 可执行文件路径，默认尝试从 PATH 查找
- `SSH Config Path`：可选，自定义 SSH config；留空则使用 `~/.ssh/config`
- `Disable Include Parsing`：是否忽略 `Include`
- `Terminal` / `Open Mode`：连接时传给 `ssht --terminal` 和 `ssht --open-mode`
