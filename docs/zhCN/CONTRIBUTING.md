# 贡献指南

> 🌐 **Language / 语言**: [English](../enUS/CONTRIBUTING.md) | [中文](CONTRIBUTING.md) | [Français](../frFR/CONTRIBUTING.md) | [Italiano](../itIT/CONTRIBUTING.md) | [日本語](../jaJP/CONTRIBUTING.md) | [Deutsch](../deDE/CONTRIBUTING.md) | [한국어](../koKR/CONTRIBUTING.md)

感谢你对 Herald 项目的关注！我们欢迎所有形式的贡献。

## 📋 目录

- [如何贡献](#如何贡献)
- [开发环境设置](#开发环境设置)
- [代码规范](#代码规范)
- [提交规范](#提交规范)
- [Pull Request 流程](#pull-request-流程)
- [问题报告与功能请求](#问题报告与功能请求)
- [Provider 开发](#provider-开发)

## 🚀 如何贡献

你可以通过以下方式贡献：

- **报告 Bug**: 在 GitHub Issues 中报告问题
- **提出功能建议**: 在 GitHub Issues 中提出新功能想法
- **提交代码**: 通过 Pull Request 提交代码改进
- **改进文档**: 帮助改进项目文档
- **回答问题**: 在 Issues 中帮助其他用户
- **开发 Provider**: 创建新的邮件或短信 provider 实现

参与本项目时，请尊重所有贡献者，接受建设性的批评，并专注于对项目最有利的事情。

## 🛠️ 开发环境设置

### 前置要求

- Go 1.25 或更高版本
- Redis（用于测试和开发）
- Git

### 快速开始

```bash
# 1. Fork 并克隆项目
git clone https://github.com/your-username/herald.git
cd herald

# 2. 添加上游仓库
git remote add upstream https://github.com/soulteary/herald.git

# 3. 安装依赖
go mod download

# 4. 运行测试
go test ./...

# 5. 启动本地服务（确保 Redis 正在运行）
# 使用 Docker Compose（推荐）
docker-compose up -d redis
go run main.go

# 或使用本地 Redis
export REDIS_ADDR=localhost:6379
go run main.go
```

### 使用 Redis 进行测试

Herald 需要 Redis 才能运行。你可以使用 Docker Compose 启动 Redis：

```bash
docker-compose up -d redis
```

或使用本地 Redis 实例：

```bash
# macOS
brew install redis
brew services start redis

# Linux
sudo apt-get install redis-server
sudo systemctl start redis
```

## 📝 代码规范

请遵循以下代码规范：

1. **遵循 Go 官方代码规范**: [Effective Go](https://go.dev/doc/effective_go)
2. **格式化代码**: 运行 `go fmt ./...`
3. **代码检查**: 使用 `golangci-lint` 或 `go vet ./...`
4. **编写测试**: 新功能必须包含测试
5. **添加注释**: 公共函数和类型必须有文档注释
6. **常量命名**: 所有常量必须使用 `ALL_CAPS` (UPPER_SNAKE_CASE) 命名风格

### 测试要求

- 所有新功能必须包含单元测试
- Provider 实现必须包含集成测试
- 测试覆盖率应保持或提高
- 提交 PR 前运行 `go test ./...`

## 📦 提交规范

### Commit Message 格式

我们使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type 类型

- `feat`: 新功能
- `fix`: 修复 Bug
- `docs`: 文档更新
- `style`: 代码格式调整（不影响代码运行）
- `refactor`: 代码重构
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 构建过程或辅助工具的变动

### 示例

```
feat(provider): 添加 SendGrid 邮件 provider 支持

实现了 SendGrid 邮件 provider，包含重试逻辑和错误处理。

Closes #123
```

```
fix(challenge): 修复 challenge 过期检查问题

修复了验证时过期 challenge 未被正确拒绝的问题。

Fixes #456
```

## 🔄 Pull Request 流程

### 创建 Pull Request

```bash
# 1. 创建功能分支
git checkout -b feature/your-feature-name

# 2. 进行更改并提交
git add .
git commit -m "feat: 添加新功能"

# 3. 同步上游代码
git fetch upstream
git rebase upstream/main

# 4. 推送分支并创建 PR
git push origin feature/your-feature-name
```

### Pull Request 检查清单

在提交 Pull Request 之前，请确保：

- [ ] 代码遵循项目代码规范
- [ ] 所有测试通过（`go test ./...`）
- [ ] 代码已格式化（`go fmt ./...`）
- [ ] 添加了必要的测试
- [ ] 更新了相关文档
- [ ] Commit message 遵循 [提交规范](#提交规范)
- [ ] 代码已通过 lint 检查
- [ ] 测试中正确处理了 Redis 依赖

所有 Pull Request 都需要经过代码审查，请及时响应审查意见。

## 🐛 问题报告与功能请求

在创建 Issue 之前，请先搜索现有的 Issues，确认问题或功能未被报告。

### Bug 报告模板

```markdown
**描述**
清晰简洁地描述 Bug。

**复现步骤**
1. 执行 '...'
2. 看到错误

**预期行为**
清晰简洁地描述你期望发生什么。

**实际行为**
清晰简洁地描述实际发生了什么。

**环境信息**
- OS: [e.g. macOS 12.0]
- Go 版本: [e.g. 1.25]
- Redis 版本: [e.g. 7.0]
- Herald 版本: [e.g. v1.0.0]
```

### 功能请求模板

```markdown
**功能描述**
清晰简洁地描述你想要的功能。

**问题描述**
这个功能解决了什么问题？为什么需要它？

**建议的解决方案**
清晰简洁地描述你希望如何实现这个功能。
```

## 🔌 Provider 开发

Herald 支持可插拔的邮件和短信 provider。如果你想添加新的 provider：

### Provider 接口

Provider 必须实现 `Provider` 接口：

```go
type Provider interface {
    Send(ctx context.Context, req *SendRequest) (*SendResponse, error)
    Name() string
}
```

### 创建新 Provider

1. **创建 provider 文件**: `internal/provider/yourprovider.go`
2. **实现 Provider 接口**: 实现 `Send()` 和 `Name()` 方法
3. **添加测试**: 创建 `internal/provider/yourprovider_test.go`
4. **注册 provider**: 在 provider 初始化中添加 provider 注册
5. **更新文档**: 记录 provider 配置选项

### Provider 最佳实践

- **幂等性**: 支持幂等键以防止重复发送
- **重试逻辑**: 为临时故障实现指数退避
- **错误处理**: 返回标准化的错误代码
- **超时处理**: 遵守上下文超时
- **日志记录**: 记录 provider 操作以便调试

### Provider 示例

参考 `internal/provider/` 中的现有 provider：
- `internal/provider/smtp.go` - SMTP 邮件 provider
- `internal/provider/sms.go` - SMS provider 接口

## 🎯 开始贡献

如果你想贡献但不知道从哪里开始，可以关注：

- 标记为 `good first issue` 的 Issues
- 标记为 `help wanted` 的 Issues
- 代码中的 `TODO` 注释
- 文档改进（修复错别字、改进清晰度、添加示例）
- Provider 实现（邮件或短信 provider）
- 测试覆盖率改进

如有问题，请查看现有的 Issues 和 Pull Requests，或在相关 Issue 中提问。

---

再次感谢你对 Herald 项目的贡献！🎉
