# Herald デプロイメントガイド

## クイックスタート

### Docker Compose の使用

```bash
cd herald
docker-compose up -d
```

### 手動デプロイメント

```bash
# ビルド
go build -o herald main.go

# 実行
./herald
```

## 設定

### 環境変数

| 変数 | 説明 | デフォルト | 必須 |
|------|------|-----------|------|
| `PORT` | サーバーポート（先頭のコロンありまたはなし、例：`8082` または `:8082`） | `:8082` | いいえ |
| `REDIS_ADDR` | Redis アドレス | `localhost:6379` | いいえ |
| `REDIS_PASSWORD` | Redis パスワード | `` | いいえ |
| `REDIS_DB` | Redis データベース | `0` | いいえ |
| `API_KEY` | 認証用の API キー | `` | 推奨 |
| `HMAC_SECRET` | セキュア認証用の HMAC シークレット | `` | オプション |
| `LOG_LEVEL` | ログレベル | `info` | いいえ |
| `CHALLENGE_EXPIRY` | チャレンジの有効期限 | `5m` | いいえ |
| `MAX_ATTEMPTS` | 最大検証試行回数 | `5` | いいえ |
| `RESEND_COOLDOWN` | 再送信のクールダウン | `60s` | いいえ |
| `CODE_LENGTH` | 検証コードの長さ | `6` | いいえ |
| `RATE_LIMIT_PER_USER` | ユーザーあたり/時間のレート制限 | `10` | いいえ |
| `RATE_LIMIT_PER_IP` | IP あたり/分のレート制限 | `5` | いいえ |
| `RATE_LIMIT_PER_DESTINATION` | 宛先あたり/時間のレート制限 | `10` | いいえ |
| `LOCKOUT_DURATION` | 最大試行回数後のユーザーロックアウト期間 | `10m` | いいえ |
| `SERVICE_NAME` | HMAC 認証用のサービス識別子 | `herald` | いいえ |
| `SMTP_HOST` | SMTP サーバーホスト | `` | 電子メール用 |
| `SMTP_PORT` | SMTP サーバーポート | `587` | 電子メール用 |
| `SMTP_USER` | SMTP ユーザー名 | `` | 電子メール用 |
| `SMTP_PASSWORD` | SMTP パスワード | `` | 電子メール用 |
| `SMTP_FROM` | SMTP 送信元アドレス | `` | 電子メール用 |
| `SMS_PROVIDER` | SMS プロバイダー | `` | SMS 用 |
| `ALIYUN_ACCESS_KEY` | 阿里云アクセスキー | `` | 阿里云 SMS 用 |
| `ALIYUN_SECRET_KEY` | 阿里云シークレットキー | `` | 阿里云 SMS 用 |
| `ALIYUN_SIGN_NAME` | 阿里云 SMS 署名名 | `` | 阿里云 SMS 用 |
| `ALIYUN_TEMPLATE_CODE` | 阿里云 SMS テンプレートコード | `` | 阿里云 SMS 用 |

## 他のサービスとの統合（オプション）

Herald は独立して動作するように設計されており、必要に応じて他のサービスと統合できます。Herald を他の認証サービスやゲートウェイサービスと統合する場合は、以下を設定できます：

**統合設定の例：**
```bash
# Herald がアクセス可能なサービス URL
export HERALD_URL=http://herald:8082

# サービス間認証用の API キー
export HERALD_API_KEY=your-secret-key

# Herald 統合を有効化（サービスがサポートしている場合）
export HERALD_ENABLED=true
```

**注意**：Herald は外部サービスの依存関係なしで単独で使用できます。他のサービスとの統合はオプションであり、特定のユースケースに依存します。

## セキュリティ

- 本番環境では HMAC 認証を使用
- 強力な API キーを設定
- 本番環境では TLS/HTTPS を使用
- レート制限を適切に設定
- Redis の不審な活動を監視
