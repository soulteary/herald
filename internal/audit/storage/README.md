# Herald 审计日志持久化存储

Herald 的审计日志持久化存储支持，提供多种存储后端和异步写入能力。

## 功能

- ✅ 支持多种存储后端（数据库、文件）
- ✅ 异步写入，不影响主流程性能
- ✅ 支持查询和分析
- ✅ 自动创建数据库表结构
- ✅ 优雅关闭，确保数据不丢失

## 存储后端

### 1. 数据库存储（PostgreSQL/MySQL）

支持 PostgreSQL 和 MySQL 数据库。

**配置**:
```bash
AUDIT_STORAGE_TYPE=database
AUDIT_DATABASE_URL=postgres://user:password@localhost:5432/herald?sslmode=disable
# 或
AUDIT_DATABASE_URL=mysql://user:password@tcp(localhost:3306)/herald
```

**数据库 Schema**:
```sql
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    challenge_id VARCHAR(100),
    user_id VARCHAR(100),
    channel VARCHAR(20),
    destination VARCHAR(255),
    purpose VARCHAR(50),
    result VARCHAR(20),
    reason VARCHAR(100),
    provider VARCHAR(50),
    provider_message_id VARCHAR(255),
    ip VARCHAR(45),
    timestamp BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_challenge_id ON audit_logs(challenge_id);
CREATE INDEX idx_audit_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_event_type ON audit_logs(event_type);
CREATE INDEX idx_audit_timestamp ON audit_logs(timestamp);
```

### 2. 文件存储（JSON Lines）

将审计日志写入文件，使用 JSON Lines 格式（每行一个 JSON 对象）。

**配置**:
```bash
AUDIT_STORAGE_TYPE=file
AUDIT_FILE_PATH=/var/log/herald/audit.log
```

**文件格式**:
```json
{"event_type":"challenge_created","challenge_id":"ch_xxx","user_id":"user_001","channel":"sms","destination":"138****8000","purpose":"login","result":"success","timestamp":1706083200}
{"event_type":"challenge_verified","challenge_id":"ch_xxx","user_id":"user_001","result":"success","timestamp":1706083260}
```

## 配置选项

| 环境变量 | 描述 | 默认值 |
|---------|------|--------|
| `AUDIT_STORAGE_TYPE` | 存储类型：`database`、`file` | 空（仅使用 Redis） |
| `AUDIT_DATABASE_URL` | 数据库连接 URL | 空 |
| `AUDIT_FILE_PATH` | 文件存储路径 | 空 |
| `AUDIT_WRITER_QUEUE_SIZE` | 异步写入队列大小 | 1000 |
| `AUDIT_WRITER_WORKERS` | 异步写入工作协程数 | 2 |

## 使用方式

### 基本配置

```bash
# 启用数据库存储
AUDIT_STORAGE_TYPE=database
AUDIT_DATABASE_URL=postgres://user:password@localhost:5432/herald?sslmode=disable

# 或启用文件存储
AUDIT_STORAGE_TYPE=file
AUDIT_FILE_PATH=/var/log/herald/audit.log
```

### 查询审计日志

```go
import "github.com/soulteary/herald/internal/audit/storage"

// 查询特定用户的审计日志
filter := &storage.QueryFilter{
    UserID: "user_001",
    Limit:  100,
}
records, err := auditManager.Query(ctx, filter)
```

## 性能考虑

- **异步写入**：审计日志通过队列异步写入，不会阻塞主流程
- **队列大小**：默认 1000，可根据负载调整
- **工作协程**：默认 2 个，可根据写入速度调整
- **批量写入**：未来可支持批量写入以提高性能

## 故障处理

- **队列满**：当队列满时，新记录会被丢弃并记录警告日志
- **存储失败**：存储失败不会影响主流程，仅记录错误日志
- **优雅关闭**：服务关闭时会等待队列中的记录写入完成（最多 10 秒）

