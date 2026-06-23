# ssht 使用指南

## 安装

Release 安装不依赖 Go，会自动选择当前平台包：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/install-release.sh | sh
```

支持 macOS Intel、macOS Apple Silicon、Linux x64 和 Linux arm64。

可选环境变量：

```bash
INSTALL_DIR=/usr/local/bin
SSHT_VERSION=v0.1.0
SSHT_REPO=Davied-H/ssht
```

源码构建安装需要 Go：

```bash
./install.sh
```

## CLI

```bash
ssht
ssht --config ~/.ssh/config
ssht --no-include
ssht --debug
ssht --doctor
ssht doctor
ssht --print-hosts
ssht --terminal terminal
ssht --open-mode tab
ssht --monitor
ssht --connect prod-api-01
```

- `--print-hosts`：以 JSON 输出发现的主机。
- `--doctor` / `ssht doctor`：检查 SSH config 健康状况并退出。
- `--connect <alias>`：跳过 TUI，直接打开指定 Host。
- `--no-include`：禁用 `Include` 递归解析。
- `--debug`：把解析 warning 输出到 stderr。
- `--terminal`：选择终端后端，支持 `auto`、`iterm`、`terminal`、`wezterm`、`kitty`、`alacritty`、`ghostty`。
- `--open-mode`：选择打开方式，支持 `auto`、`window`、`tab`。
- `--monitor`：启动时显示 SSH 监控面板。

也可以通过环境变量设置默认行为：

```bash
SSHT_TERMINAL=iterm ssht
SSHT_OPEN_MODE=tab ssht
```

## 快捷键

- `/`：进入搜索并编辑当前筛选
- `Ctrl+U`：在搜索中清空筛选
- `tag:<name>`：按标签筛选
- `user:<name>` / `port:<port>` / `group:<name>` / `jump:<name>` / `file:<name>`：结构化筛选
- `-<term>`：否定筛选，例如 `prod -db`
- `fav:`：只显示收藏主机
- `recent:`：只显示最近连接主机
- `:` / `Ctrl+K`：打开命令面板
- `[` / `]` 或 Left / Right：切换分组
- `PgUp` / `PgDn` / `Home` / `End`：快速移动光标
- `Tab`：在主机列表与分组侧栏之间切换焦点
- `Enter`：打开选中主机
- `Space`：标记或取消标记主机
- `f`：收藏或取消收藏
- `e`：编辑主机
- `A`：新增主机
- `d`：删除主机
- `Ctrl+S`：在新增/编辑表单中进入保存确认
- `s`：确认待执行的写入操作
- `g`：移动当前主机或已标记主机到分组
- `r`：重新加载 SSH config
- `R`：刷新当前主机 monitor 快照
- `H`：查看本地连接历史
- `W`：查看 SSH config 解析 warning
- `?`：帮助
- `q` / `Esc`：退出

侧栏聚焦后：

- `j` / `k`：移动分组选择
- `a`：新建空 group
- `r`：重命名当前 group
- `m`：合并 group
- `d`：删除当前 group
- `M`：把已标记主机批量移动到当前 group
- `J` / `K`：调整 group 顺序

## SSH Config 支持范围

`ssht` 会发现具体可连接的 `Host` 别名，并提取这些字段用于展示：

- `HostName`
- `User`
- `Port`
- `IdentityFile`
- `ProxyJump`
- `ProxyCommand`

`Include` 支持相对路径、`~`、环境变量和 glob。通配或否定模式不会作为可直接连接条目展示，例如 `Host *`、`Host *.internal` 和 `Host !blocked`。

实际连接始终执行：

```bash
ssh <alias>
```

所以 `Match`、token 展开、最终生效选项等复杂语义仍由 OpenSSH 处理。

## 配置健康检查

```bash
ssht doctor
ssht --doctor --config ~/.ssh/config
```

健康检查会复用 `ssht` 的解析结果并输出重复 Host alias、解析 warning、无效端口、空 HostName、重复 tags 等问题。发现 error 时进程以非 0 状态退出；warning 和 info 只作为提示。

## 分组、标签和隐藏

元数据写在 `Host` 前面的注释里：

```sshconfig
# ssht: group=prod tags=api,critical
Host prod-api-01
    HostName 192.0.2.12
    User deploy
```

隐藏不想展示的 Host：

```sshconfig
# ssht: hidden=true
Host git-example
    HostName git.example.com
    User git
```

兼容 `sshm` 风格的 sshpass 注释：

```sshconfig
# sshpass 密码: example-password
Host password-host
    HostName 192.0.2.12
    User root
```

密码只在内存中用于连接命令，不会出现在 `--print-hosts` JSON 输出中。

## 本地状态

`ssht` 会保存收藏、最近连接时间和连接次数。不会保存 HostName、User、IdentityFile、密码、私钥或其他 SSH 凭据。

状态文件路径：

- 设置了 `XDG_STATE_HOME`：`$XDG_STATE_HOME/ssht/state.json`
- macOS 默认：`~/Library/Application Support/ssht/state.json`
- 其他系统默认：`~/.local/state/ssht/state.json`

## 终端行为

默认按以下顺序自动选择第一个可用终端：

1. iTerm2
2. Terminal.app
3. WezTerm
4. kitty
5. Alacritty
6. Ghostty

`--open-mode auto` 在 iTerm2 会话内会使用右侧分屏；不在 iTerm2 会话内时按新 tab 行为打开。显式指定 `tab` 或 `window` 后不会自动切换语义。

如果 `Host` 前紧挨着 `# sshpass 密码: ...` 注释，则连接命令会变成：

```bash
sshpass -p <password> ssh <alias>
```

## Raycast 扩展

Raycast extension 位于 `raycast/`。它会调用：

- `ssht --print-hosts`
- `ssht --connect <alias>`

首次使用：

```bash
cd raycast
npm install
npm run dev
```

如果 Raycast 找不到 `ssht`，先运行 `./install.sh` 或 `scripts/install-release.sh`，然后在 extension preferences 里把 `ssht Path` 设置为安装后的绝对路径。

## Codex / Claude Code

仓库包含：

- `AGENTS.md`：Codex 项目说明
- `CLAUDE.md`：Claude Code 项目说明
- `skills/ssht-config-auditor`
- `skills/ssht-release-packager`
- `skills/ssht-raycast-helper`

安装到本机 agent 目录：

```bash
sh scripts/configure-agents.sh
```

远程一键安装：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/ssht/main/scripts/configure-agents.sh | sh
```

## 发布和打包

仓库内置 GitHub Actions：

- `CI`：运行 Go 测试并构建 Raycast extension。
- `Release`：推送 `v*` tag 或手动触发时构建发布包、生成 `checksums.txt` 并上传到 GitHub Release。

正式发布：

```bash
git tag v1.0.0
git push github v1.0.0
```

Release workflow 构建：

- `ssht_<version>_darwin_amd64.tar.gz`
- `ssht_<version>_darwin_arm64.tar.gz`
- `ssht_<version>_linux_amd64.tar.gz`
- `ssht_<version>_linux_arm64.tar.gz`
- `ssht_<version>_windows_amd64.zip`
- `checksums.txt`

## 公开前检查

发布前运行仓库内置 preflight：

```bash
sh scripts/preflight-release.sh
```

这条命令会检查：

- Go 测试：`go test ./...`
- 敏感信息：私钥、常见 token、云密钥、个人绝对路径
- README 和 usage 示例：IP 必须使用 RFC 5737 文档网段，HostName 域名必须使用 `example.com`
- Release 产物命名：workflow、安装脚本和本文档中的命名规则必须一致

文档和测试示例使用：

- IP：`192.0.2.0/24`、`198.51.100.0/24`、`203.0.113.0/24`
- 域名：`example.com`
- 用户：`deploy`、`demo`、`git`
