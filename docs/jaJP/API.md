# Herald API ドキュメント

Herald は、SMS と電子メールを介して検証コードを送信する検証コードおよび OTP サービスで、組み込みのレート制限とセキュリティ制御を備えています。

## ベース URL

```
http://localhost:8082
```

## 認証

Herald は次の優先順位で 3 つの認証方法をサポートしています：

1. **mTLS**（最も安全）：クライアント証明書検証による相互 TLS（最高優先度）
2. **HMAC 署名**（セキュア）：`X-Signature`、`X-Timestamp`、`X-Service` ヘッダーを設定
3. **API キー**（シンプル）：`X-API-Key` ヘッダーを設定（最低優先度）

### mTLS 認証

検証済みのクライアント証明書で HTTPS を使用する場合、Herald は自動的に mTLS 経由でリクエストを認証します。これは最も安全な方法で、他の認証方法よりも優先されます。

### HMAC 署名

HMAC 署名は次のように計算されます：
```
HMAC-SHA256(timestamp:service:body, secret)
```

ここで：
- `timestamp`：Unix タイムスタンプ（秒）
- `service`：サービス識別子（例："my-service"、"api-gateway"）
- `body`：リクエストボディ（JSON 文字列）
- `secret`：HMAC シークレットキー

**注意**：リプレイ攻撃を防ぐため、タイムスタンプはサーバー時刻の 5 分（300 秒）以内である必要があります。タイムスタンプウィンドウは設定可能ですが、デフォルトは 5 分です。

**注意**：現在、キーローテーション用の `X-Key-Id` ヘッダーはサポートされていません。この機能は将来のバージョンで計画されています。

## エンドポイント

### ヘルスチェック

**GET /healthz**

サービスの健全性を確認します。

**レスポンス：**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### チャレンジの作成

**POST /v1/otp/challenges**

新しい検証チャレンジを作成し、検証コードを送信します。

**リクエスト：**
```json
{
  "user_id": "u_123",
  "channel": "sms",
  "destination": "+8613800138000",
  "purpose": "login",
  "locale": "zh-CN",
  "client_ip": "192.168.1.1",
  "ua": "Mozilla/5.0..."
}
```

**レスポンス：**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

**エラーレスポンス：**

すべてのエラーレスポンスは次の形式に従います：
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "オプションのエラーメッセージ"
}
```

可能なエラーコード：
- `invalid_request`：リクエストボディの解析に失敗
- `user_id_required`：必須フィールド `user_id` が欠落
- `invalid_channel`：無効なチャネルタイプ（"sms"、"email" または "dingtalk" である必要があります）
- `destination_required`：必須フィールド `destination` が欠落
- `rate_limit_exceeded`：レート制限を超過
- `resend_cooldown`：再送信のクールダウン期間が終了していない
- `user_locked`：ユーザーが一時的にロックされています
- `internal_error`：内部サーバーエラー

HTTP ステータスコード：
- `400 Bad Request`：無効なリクエストパラメータ
- `401 Unauthorized`：認証に失敗
- `403 Forbidden`：ユーザーがロックされています
- `429 Too Many Requests`：レート制限を超過
- `500 Internal Server Error`：内部サーバーエラー

### チャレンジの検証

**POST /v1/otp/verifications**

チャレンジコードを検証します。

**リクエスト：**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**レスポンス（成功）：**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**レスポンス（失敗）：**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**エラーレスポンス：**

可能なエラーコード：
- `invalid_request`：リクエストボディの解析に失敗
- `challenge_id_required`：必須フィールド `challenge_id` が欠落
- `code_required`：必須フィールド `code` が欠落
- `invalid_code_format`：検証コードの形式が無効
- `expired`：チャレンジが期限切れ
- `invalid`：無効な検証コード
- `locked`：試行回数が多すぎるため、チャレンジがロックされています
- `verification_failed`：一般的な検証失敗
- `internal_error`：内部サーバーエラー

HTTP ステータスコード：
- `400 Bad Request`：無効なリクエストパラメータ
- `401 Unauthorized`：検証に失敗
- `403 Forbidden`：ユーザーがロックされています
- `500 Internal Server Error`：内部サーバーエラー

### チャレンジの取り消し

**POST /v1/otp/challenges/{id}/revoke**

チャレンジを取り消します（オプション）。

**レスポンス（成功）：**
```json
{
  "ok": true
}
```

**レスポンス（失敗）：**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**エラーレスポンス：**

可能なエラーコード：
- `challenge_id_required`：URL パラメータにチャレンジ ID が欠落
- `internal_error`：内部サーバーエラー

HTTP ステータスコード：
- `400 Bad Request`：無効なリクエスト
- `500 Internal Server Error`：内部サーバーエラー

## レート制限

Herald は多次元レート制限を実装しています：

- **ユーザーごと**：1 時間あたり 10 リクエスト（設定可能）
- **IP ごと**：1 分あたり 5 リクエスト（設定可能）
- **宛先ごと**：1 時間あたり 10 リクエスト（設定可能）
- **再送信のクールダウン**：再送信の間隔は 60 秒

## エラーコード

このセクションでは、API が返す可能性のあるすべてのエラーコードをリストします。

### リクエスト検証エラー
- `invalid_request`：リクエストボディの解析に失敗または無効な JSON
- `user_id_required`：必須フィールド `user_id` が欠落
- `invalid_channel`：無効なチャネルタイプ（"sms"、"email" または "dingtalk" である必要があります）
- `destination_required`：必須フィールド `destination` が欠落
- `challenge_id_required`：必須フィールド `challenge_id` が欠落
- `code_required`：必須フィールド `code` が欠落
- `invalid_code_format`：検証コードの形式が無効

### 認証エラー
- `authentication_required`：有効な認証が提供されていません
- `invalid_timestamp`：無効なタイムスタンプ形式
- `timestamp_expired`：タイムスタンプが許可されたウィンドウ（5 分）の外にあります
- `invalid_signature`：HMAC 署名の検証に失敗

### チャレンジエラー
- `expired`：チャレンジが期限切れ
- `invalid`：無効な検証コード
- `locked`：試行回数が多すぎるため、チャレンジがロックされています
- `too_many_attempts`：失敗した試行回数が多すぎます（`locked` に含まれる場合があります）
- `verification_failed`：一般的な検証失敗

### レート制限エラー
- `rate_limit_exceeded`：レート制限を超過
- `resend_cooldown`：再送信のクールダウン期間が終了していない

### ユーザーステータスエラー
- `user_locked`：ユーザーが一時的にロックされています

### システムエラー
- `internal_error`：内部サーバーエラー
