# Contributing Guide

> 🌐 **Language / 语言**: [English](CONTRIBUTING.md) | [中文](../zhCN/CONTRIBUTING.md) | [Français](../frFR/CONTRIBUTING.md) | [Italiano](../itIT/CONTRIBUTING.md) | [日本語](../jaJP/CONTRIBUTING.md) | [Deutsch](../deDE/CONTRIBUTING.md) | [한국어](../koKR/CONTRIBUTING.md)

Thank you for your interest in the Herald project! We welcome all forms of contributions.

## 📋 Table of Contents

- [How to Contribute](#how-to-contribute)
- [Development Environment Setup](#development-environment-setup)
- [Code Standards](#code-standards)
- [Commit Standards](#commit-standards)
- [Pull Request Process](#pull-request-process)
- [Bug Reports and Feature Requests](#bug-reports-and-feature-requests)
- [Provider Development](#provider-development)

## 🚀 How to Contribute

You can contribute in the following ways:

- **Report Bugs**: Report issues in GitHub Issues
- **Suggest Features**: Propose new feature ideas in GitHub Issues
- **Submit Code**: Submit code improvements via Pull Requests
- **Improve Documentation**: Help improve project documentation
- **Answer Questions**: Help other users in Issues
- **Develop Providers**: Create new email or SMS provider implementations

When participating in this project, please respect all contributors, accept constructive criticism, and focus on what's best for the project.

## 🛠️ Development Environment Setup

### Prerequisites

- Go 1.25 or higher
- Redis (for testing and development)
- Git

### Quick Start

```bash
# 1. Fork and clone the project
git clone https://github.com/your-username/herald.git
cd herald

# 2. Add upstream repository
git remote add upstream https://github.com/soulteary/herald.git

# 3. Install dependencies
go mod download

# 4. Run tests
go test ./...

# 5. Start local service (ensure Redis is running)
# Using Docker Compose (recommended)
docker-compose up -d redis
go run main.go

# Or using local Redis
export REDIS_ADDR=localhost:6379
go run main.go
```

### Testing with Redis

Herald requires Redis for operation. You can use Docker Compose to start Redis:

```bash
docker-compose up -d redis
```

Or use a local Redis instance:

```bash
# macOS
brew install redis
brew services start redis

# Linux
sudo apt-get install redis-server
sudo systemctl start redis
```

## 📝 Code Standards

Please follow these code standards:

1. **Follow Go Official Code Standards**: [Effective Go](https://go.dev/doc/effective_go)
2. **Format Code**: Run `go fmt ./...`
3. **Code Checking**: Use `golangci-lint` or `go vet ./...`
4. **Write Tests**: New features must include tests
5. **Add Comments**: Public functions and types must have documentation comments
6. **Constant Naming**: All constants must use `ALL_CAPS` (UPPER_SNAKE_CASE) naming style

### Testing Requirements

- All new features must include unit tests
- Provider implementations must include integration tests
- Test coverage should be maintained or improved
- Run `go test ./...` before submitting PRs

## 📦 Commit Standards

### Commit Message Format

We use [Conventional Commits](https://www.conventionalcommits.org/) standard:

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation update
- `style`: Code format adjustment (doesn't affect code execution)
- `refactor`: Code refactoring
- `perf`: Performance optimization
- `test`: Test related
- `chore`: Build process or auxiliary tool changes

### Examples

```
feat(provider): Add SendGrid email provider support

Implemented SendGrid email provider with retry logic and error handling.

Closes #123
```

```
fix(challenge): Fix challenge expiration check issue

Fixed the issue where expired challenges were not properly rejected during verification.

Fixes #456
```

## 🔄 Pull Request Process

### Create Pull Request

```bash
# 1. Create feature branch
git checkout -b feature/your-feature-name

# 2. Make changes and commit
git add .
git commit -m "feat: Add new feature"

# 3. Sync upstream code
git fetch upstream
git rebase upstream/main

# 4. Push branch and create PR
git push origin feature/your-feature-name
```

### Pull Request Checklist

Before submitting a Pull Request, please ensure:

- [ ] Code follows project code standards
- [ ] All tests pass (`go test ./...`)
- [ ] Code is formatted (`go fmt ./...`)
- [ ] Necessary tests are added
- [ ] Related documentation is updated
- [ ] Commit message follows [Commit Standards](#commit-standards)
- [ ] Code passes lint checks
- [ ] Redis dependency is properly handled in tests

All Pull Requests require code review. Please respond to review comments promptly.

## 🐛 Bug Reports and Feature Requests

Before creating an Issue, please search existing Issues to confirm the problem or feature hasn't been reported.

### Bug Report Template

```markdown
**Description**
Clearly and concisely describe the bug.

**Reproduction Steps**
1. Execute '...'
2. See error

**Expected Behavior**
Clearly and concisely describe what you expected to happen.

**Actual Behavior**
Clearly and concisely describe what actually happened.

**Environment Information**
- OS: [e.g. macOS 12.0]
- Go Version: [e.g. 1.25]
- Redis Version: [e.g. 7.0]
- Herald Version: [e.g. v1.0.0]
```

### Feature Request Template

```markdown
**Feature Description**
Clearly and concisely describe the feature you want.

**Problem Description**
What problem does this feature solve? Why is it needed?

**Proposed Solution**
Clearly and concisely describe how you hope to implement this feature.
```

## 🔌 Provider Development

Herald supports pluggable email and SMS providers. If you want to add a new provider:

### Provider Interface

Providers must implement the `Provider` interface:

```go
type Provider interface {
    Send(ctx context.Context, req *SendRequest) (*SendResponse, error)
    Name() string
}
```

### Creating a New Provider

1. **Create provider file**: `internal/provider/yourprovider.go`
2. **Implement Provider interface**: Implement `Send()` and `Name()` methods
3. **Add tests**: Create `internal/provider/yourprovider_test.go`
4. **Register provider**: Add provider registration in provider initialization
5. **Update documentation**: Document provider configuration options

### Provider Best Practices

- **Idempotency**: Support idempotency keys to prevent duplicate sends
- **Retry Logic**: Implement exponential backoff for transient failures
- **Error Handling**: Return normalized error codes
- **Timeout Handling**: Respect context timeouts
- **Logging**: Log provider operations for debugging

### Example Provider

See existing providers in `internal/provider/` for reference:
- `internal/provider/smtp.go` - SMTP email provider
- `internal/provider/sms.go` - SMS provider interface

## 🎯 Getting Started

If you want to contribute but don't know where to start, you can focus on:

- Issues labeled `good first issue`
- Issues labeled `help wanted`
- `TODO` comments in code
- Documentation improvements (fix typos, improve clarity, add examples)
- Provider implementations (email or SMS providers)
- Test coverage improvements

If you have questions, please check existing Issues and Pull Requests, or ask in relevant Issues.

---

Thank you again for contributing to the Herald project! 🎉
