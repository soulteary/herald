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

**コードベースと一致した完全な一覧：** [English](enUS/DEPLOYMENT.md#environment-variables) | [中文](zhCN/DEPLOYMENT.md#环境变量)

| 変数 | 説明 | デフォルト | 必須 |
|------|------|-----------|------|
| `PORT` | サーバーポート（先頭のコロンありまたはなし、例：`8082` または `:8082`） | `:8082` | いいえ |
| `REDIS_ADDR` | Redis アドレス | `localhost:6379` | いいえ |
| `REDIS_PASSWORD` | Redis パスワード | `` | いいえ |
| `REDIS_DB` | Redis データベース | `0` | いいえ |
| `API_KEY` | 認証用の API キー | `` | 推奨 |
| `HMAC_SECRET` | セキュア認証用の HMAC シークレット | `` | オプション |
| `HERALD_HMAC_KEYS` | 複数 HMAC キー（JSON） | `` | オプション |
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
| `SMTP_HOST` | SMTP サーバーホスト | `` | 電子メール用（組み込み） |
| `SMTP_PORT` | SMTP サーバーポート | `587` | 電子メール用 |
| `SMTP_USER` | SMTP ユーザー名 | `` | 電子メール用 |
| `SMTP_PASSWORD` | SMTP パスワード | `` | 電子メール用 |
| `SMTP_FROM` | SMTP 送信元アドレス | `` | 電子メール用 |
| `SMS_PROVIDER` | SMS プロバイダー（ログ用など） | `` | SMS 用 |
| `SMS_API_BASE_URL` | SMS HTTP API ベース URL | `` | SMS（HTTP API）用 |
| `SMS_API_KEY` | SMS API キー | `` | SMS 用（オプション） |
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) のベース URL（例：`http://herald-dingtalk:8083`） | `` | DingTalk チャネル用 |
| `HERALD_DINGTALK_API_KEY` | オプションの API キー；herald-dingtalk の `API_KEY` と一致させる必要あり（設定時） | `` | なし |
| `HERALD_SMTP_API_URL` | [herald-smtp](https://github.com/soulteary/herald-smtp) のベース URL（例：`http://herald-smtp:8084`）；設定時は組み込み SMTP は使用されない | `` | 電子メールチャネル用（オプション） |
| `HERALD_SMTP_API_KEY` | オプションの API キー；herald-smtp の `API_KEY` と一致させる必要あり（設定時） | `` | なし |
| `HERALD_TEST_MODE` | `true` の場合：Redis/レスポンスに debug 用コード。**テスト専用；本番では必ず `false`。** | `false` | いいえ |

### 電子メールチャネル（herald-smtp）

`HERALD_SMTP_API_URL` を設定すると、Herald は組み込み SMTP を使用しません。メール送信は HTTP で [herald-smtp](https://github.com/soulteary/herald-smtp) に転送されます。すべての SMTP 認証情報とロジックは herald-smtp にあり、このモードでは Herald は電子メールチャネル用の SMTP 認証情報を保存しません。`HERALD_SMTP_API_URL` を herald-smtp サービスのベース URL に設定してください。herald-smtp で `API_KEY` を設定している場合は、`HERALD_SMTP_API_KEY` を同じ値に設定してください。`HERALD_SMTP_API_URL` 設定時、Herald は `SMTP_HOST` および関連する組み込み SMTP 設定を無視します。

### DingTalk チャネル（herald-dingtalk）

`channel` が `dingtalk` の場合、Herald は自身でメッセージを送信せず、[herald-dingtalk](https://github.com/soulteary/herald-dingtalk) に HTTP で転送します。DingTalk の認証情報とビジネスロジックはすべて herald-dingtalk にあり、Herald は DingTalk 認証情報を保存しません。`HERALD_DINGTALK_API_URL` を herald-dingtalk サービスのベース URL に設定してください。herald-dingtalk で `API_KEY` を設定している場合は、`HERALD_DINGTALK_API_KEY` を同じ値に設定してください。

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
