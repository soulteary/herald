# Herald - OTP および検証コードサービス

> **📧 安全な検証へのゲートウェイ**

## 🌐 多言語ドキュメント

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald は、電子メールと SMS を介して検証コードを送信できる本番対応のスタンドアロン OTP および検証コードサービスです。組み込みのレート制限、セキュリティ制御、監査ログ記録機能を備えています。Herald は独立して動作するように設計されており、必要に応じて他のサービスと統合できます。

## 主な機能

- 🔒 **セキュリティ設計**：Argon2 ハッシュストレージを使用したチャレンジベースの検証、複数の認証方法（mTLS、HMAC、API Key）
- 📊 **組み込みレート制限**：多次元レート制限（ユーザーごと、IP ごと、宛先ごと）、設定可能なしきい値
- 📝 **完全な監査証跡**：プロバイダー追跡を含むすべての操作の完全な監査ログ記録
- 🔌 **プラガブルプロバイダー**：拡張可能な電子メールおよび SMS プロバイダーアーキテクチャ

## クイックスタート

### Docker Compose の使用

最も簡単な方法は、Redis を含む Docker Compose を使用することです：

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

期待される応答：
```json
{
  "status": "ok",
  "service": "herald"
}
```

### API のテスト

テストチャレンジを作成（認証が必要 - [API ドキュメント](docs/jaJP/API.md) を参照）：

```bash
# Set your API key (from docker-compose.yml: your-secret-api-key-here)
export API_KEY="your-secret-api-key-here"

# Create a challenge
curl -X POST http://localhost:8082/v1/otp/challenges \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "channel": "email",
    "destination": "user@example.com",
    "purpose": "login"
  }'
```

### ログの表示

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### 手動デプロイ

手動デプロイと高度な設定については、[デプロイメントガイド](docs/jaJP/DEPLOYMENT.md) を参照してください。

## 基本設定

Herald を開始するには最小限の設定が必要です：

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

レート制限、チャレンジの有効期限、プロバイダー設定を含む完全な設定オプションについては、[デプロイメントガイド](docs/jaJP/DEPLOYMENT.md#configuration) を参照してください。

## ドキュメント

### 開発者向け

- **[API ドキュメント](docs/jaJP/API.md)** - 認証方法、エンドポイント、エラーコードを含む完全な API リファレンス
- **[デプロイメントガイド](docs/jaJP/DEPLOYMENT.md)** - 設定オプション、Docker デプロイメント、統合例

### 運用向け

- **[監視ガイド](docs/jaJP/MONITORING.md)** - Prometheus メトリクス、Grafana ダッシュボード、アラート
- **[トラブルシューティングガイド](docs/jaJP/TROUBLESHOOTING.md)** - 一般的な問題、診断手順、解決策

### ドキュメントインデックス

すべてのドキュメントの完全な概要については、[docs/jaJP/README.md](docs/jaJP/README.md) を参照してください。

## License

See [LICENSE](LICENSE) for details.
