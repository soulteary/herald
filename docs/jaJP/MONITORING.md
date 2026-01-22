# Herald モニタリングガイド

Herald は、モニタリングと可観測性のために Prometheus メトリクスを公開します。このドキュメントでは、利用可能なすべてのメトリクス、モニタリングの設定方法、およびベストプラクティスについて説明します。

## メトリクスエンドポイント

Herald は `/metrics` エンドポイントで Prometheus メトリクスを公開します：

```
GET http://localhost:8082/metrics
```

## 利用可能なメトリクス

### チャレンジメトリクス

#### `herald_otp_challenges_total`

作成された OTP チャレンジの総数を追跡するカウンター。

**ラベル：**
- `channel`: チャネルタイプ（`sms` または `email`）
- `purpose`: チャレンジの目的（例：`login`、`reset`、`bind`）
- `result`: 操作の結果（`success` または `failed`）

**例：**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### 送信メトリクス

#### `herald_otp_sends_total`

プロバイダー経由で送信された OTP の総数を追跡するカウンター。

**ラベル：**
- `channel`: チャネルタイプ（`sms` または `email`）
- `provider`: プロバイダー名（例：`smtp`、`aliyun`、`placeholder`）
- `result`: 送信操作の結果（`success` または `failed`）

**例：**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

OTP 送信操作の継続時間を追跡するヒストグラム。

**ラベル：**
- `provider`: プロバイダー名

**バケット：** デフォルトの Prometheus バケット（0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10）

**例：**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### 検証メトリクス

#### `herald_otp_verifications_total`

OTP 検証の総数を追跡するカウンター。

**ラベル：**
- `result`: 検証結果（`success` または `failed`）
- `reason`: 失敗理由（例：`expired`、`invalid`、`locked`、`too_many_attempts`）

**例：**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### レート制限メトリクス

#### `herald_rate_limit_hits_total`

レート制限ヒットの総数を追跡するカウンター。

**ラベル：**
- `scope`: レート制限スコープ（`user`、`ip`、`destination`、`resend_cooldown`）

**例：**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Redis メトリクス

#### `herald_redis_latency_seconds`

Redis 操作のレイテンシを追跡するヒストグラム。

**ラベル：**
- `operation`: Redis 操作タイプ（`get`、`set`、`del`、`exists`）

**バケット：** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**例：**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Prometheus 設定

### 基本スクレイプ設定

`prometheus.yml` に以下を追加します：

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
```

### サービスディスカバリー

Kubernetes デプロイメントの場合、サービスディスカバリーを使用します：

```yaml
scrape_configs:
  - job_name: 'herald'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - default
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: herald
        action: keep
      - source_labels: [__meta_kubernetes_pod_ip]
        target_label: __address__
        replacement: '${1}:8082'
```

## 監視すべき主要メトリクス

### ビジネスメトリクス

1. **チャレンジ作成率**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **チャレンジ成功率**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **検証成功率**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **プロバイダー別送信成功率**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### パフォーマンスメトリクス

1. **平均送信継続時間**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **P95 送信継続時間**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Redis P99 レイテンシ**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### エラーメトリクス

1. **レート制限ヒット率**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **検証失敗理由**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **送信失敗率**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## アラートルール

### アラートルールの例

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # 高失敗率
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald 検証失敗率が高い"
          description: "検証失敗率は {{ $value | humanizePercentage }} です"

      # 高送信失敗率
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald 送信失敗率が高い"
          description: "送信失敗率は {{ $value | humanizePercentage }} です"

      # 高 Redis レイテンシ
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald Redis レイテンシが高い"
          description: "P99 Redis レイテンシは {{ $value }}s です"

      # 高レート制限ヒット
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Herald レート制限ヒットが高い"
          description: "レート制限ヒット率は {{ $value }} ヒット/秒です"
```

## Grafana ダッシュボード

### 推奨ダッシュボードパネル

1. **概要パネル**
   - チャレンジ作成率
   - 検証成功率
   - 送信成功率
   - 現在のアクティブチャレンジ数（追跡されている場合）

2. **パフォーマンスパネル**
   - 送信継続時間（P50、P95、P99）
   - Redis レイテンシ（P50、P95、P99）
   - リクエスト率

3. **エラーパネル**
   - 検証失敗理由の内訳
   - プロバイダー別送信失敗率
   - スコープ別レート制限ヒット

4. **プロバイダーパネル**
   - プロバイダー別送信成功率
   - プロバイダー別送信継続時間
   - プロバイダー別送信量

## OpenTelemetry サポート

**注意**：分散トレーシングのための OpenTelemetry サポートは計画されていますが、まだ実装されていません。これには以下が含まれます：
- `traceparent` / `tracestate` ヘッダー伝播
- コアスパン：`otp.challenge.create`、`otp.provider.send`、`otp.verify`
- スパンタグ：`channel`、`purpose`、`provider`、`result`、`reason`

実装タイムラインについては、プロジェクトのロードマップを参照してください。

## ベストプラクティス

1. **主要ビジネスメトリクスを監視**: チャレンジ作成、検証成功率、送信成功率を追跡
2. **アラートを設定**: 高失敗率とパフォーマンス低下のアラートを設定
3. **プロバイダーパフォーマンスを追跡**: プロバイダー別の送信継続時間と成功率を監視
4. **Redis の健全性を監視**: Redis レイテンシと接続問題を追跡
5. **レート制限監視**: レート制限ヒットを監視して使用パターンと潜在的な悪用を理解

## 関連ドキュメント

- [API ドキュメント](API.md) - API エンドポイントの詳細
- [デプロイメントガイド](DEPLOYMENT.md) - デプロイメントと設定
- [トラブルシューティングガイド](TROUBLESHOOTING.md) - 一般的な問題と解決策
