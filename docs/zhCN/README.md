# 文档索引

欢迎查阅 Herald OTP 和验证码服务的文档。

## 🌐 多语言文档 / Multi-language Documentation

- [English](../enUS/README.md) | [中文](README.md) | [Français](../frFR/README.md) | [Italiano](../itIT/README.md) | [日本語](../jaJP/README.md) | [Deutsch](../deDE/README.md) | [한국어](../koKR/README.md)

## 📚 文档列表

### 核心文档

- **[README.md](../../README.zhCN.md)** - 项目概述和快速开始指南

### 详细文档

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - 全栈架构说明
  - Stargate / Warden / Herald / Redis / Provider
  - 数据流与关键接口（challenge、verify、provider send）
  - 安全边界（HMAC/mTLS、PII）

- **[API.md](API.md)** - 完整的 API 端点文档
  - 认证方法
  - 健康检查端点
  - 挑战创建和验证
  - 速率限制
  - 错误代码和响应

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - 部署指南
  - Docker Compose 部署
  - 手动部署
  - 配置选项
  - 与其他服务的可选集成
  - 安全最佳实践

- **[MONITORING.md](MONITORING.md)** - 监控指南
  - Prometheus 指标
  - Grafana 仪表板
  - 告警规则
  - 最佳实践

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - 故障排查指南
  - 常见问题和解决方案
  - 诊断步骤
  - 性能优化

## 🚀 快速导航

### 新手入门

1. 阅读 [README.zhCN.md](../../README.zhCN.md) 了解项目
2. 查看 [快速开始](../../README.zhCN.md#快速开始) 部分
3. 参考 [配置说明](../../README.zhCN.md#配置说明) 配置服务

### 开发人员

1. 阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 了解全栈架构
2. 查看 [API.md](API.md) 了解 API 接口
3. 参考 [DEPLOYMENT.md](DEPLOYMENT.md) 了解部署选项

### 运维人员

1. 阅读 [DEPLOYMENT.md](DEPLOYMENT.md) 了解部署方式
2. 查看 [API.md](API.md) 了解 API 端点详情
3. 参考 [安全](DEPLOYMENT.md#安全) 了解安全最佳实践
4. 监控服务健康状态：[MONITORING.md](MONITORING.md)
5. 排查问题：[TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 文档结构

```
herald/
├── README.md              # 项目主文档（英文）
├── README.zhCN.md         # 项目主文档（中文）
├── README.deDE.md         # 项目主文档（德文）
├── README.frFR.md         # 项目主文档（法文）
├── README.itIT.md         # 项目主文档（意大利文）
├── README.jaJP.md         # 项目主文档（日文）
├── README.koKR.md         # 项目主文档（韩文）
├── docs/
│   ├── enUS/              # English
│   ├── zhCN/              # 中文（本文件）
│   ├── deDE/              # Deutsch
│   ├── frFR/              # Français
│   ├── itIT/              # Italiano
│   ├── jaJP/              # 日本語
│   └── koKR/              # 한국어
│       # 各语言目录均包含：
│       # README.md、ARCHITECTURE.md、API.md、DEPLOYMENT.md、
│       # MONITORING.md、SECURITY.md、CONTRIBUTING.md、TROUBLESHOOTING.md
└── ...
```

## 🔍 按主题查找

### 架构相关

- 全栈概览：[ARCHITECTURE.md](ARCHITECTURE.md)
- 数据流与关键接口：[ARCHITECTURE.md#数据流与关键接口](ARCHITECTURE.md#数据流与关键接口)
- 安全边界：[ARCHITECTURE.md#安全边界](ARCHITECTURE.md#安全边界)

### API 相关

- API 端点列表：[API.md](API.md)
- 认证方法：[API.md#认证](API.md#认证)
- 错误处理：[API.md#错误代码](API.md#错误代码)
- 速率限制：[API.md#速率限制](API.md#速率限制)

### 部署相关

- Docker 部署：[DEPLOYMENT.md#快速开始](DEPLOYMENT.md#快速开始)
- 配置选项：[DEPLOYMENT.md#配置](DEPLOYMENT.md#配置)
- 服务集成：[DEPLOYMENT.md#与其他服务集成可选](DEPLOYMENT.md#与其他服务集成可选)
- 安全：[DEPLOYMENT.md#安全](DEPLOYMENT.md#安全)

### 监控和运维

- Prometheus 指标：[MONITORING.md](MONITORING.md)
- Grafana 仪表板：[MONITORING.md#grafana-仪表板](MONITORING.md#grafana-仪表板)
- 故障排查：[TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 使用建议

1. **首次使用**：从 [README.zhCN.md](../../README.zhCN.md) 开始，按照快速开始指南操作
2. **配置服务**：参考 [DEPLOYMENT.md](DEPLOYMENT.md) 了解所有配置选项
3. **集成服务**：查看 [DEPLOYMENT.md](DEPLOYMENT.md) 中的集成部分
4. **API 集成**：阅读 [API.md](API.md) 了解 API 接口
5. **监控服务**：使用 [MONITORING.md](MONITORING.md) 设置监控
6. **排查问题**：参考 [TROUBLESHOOTING.md](TROUBLESHOOTING.md) 了解常见问题

## 📝 文档更新

文档会随着项目的发展持续更新。如果发现文档有误或需要补充，欢迎提交 Issue 或 Pull Request。

## 🤝 贡献

欢迎贡献文档改进：

1. 发现错误或需要改进的地方
2. 提交 Issue 描述问题
3. 或直接提交 Pull Request
