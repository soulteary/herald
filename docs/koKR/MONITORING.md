# Herald 모니터링 가이드

Herald는 모니터링 및 관찰 가능성을 위해 Prometheus 메트릭을 노출합니다. 이 문서에서는 사용 가능한 모든 메트릭, 모니터링 구성 방법 및 모범 사례에 대해 설명합니다.

## 메트릭 엔드포인트

Herald는 `/metrics` 엔드포인트에서 Prometheus 메트릭을 노출합니다:

```
GET http://localhost:8082/metrics
```

## 사용 가능한 메트릭

### 챌린지 메트릭

#### `herald_otp_challenges_total`

생성된 OTP 챌린지의 총 수를 추적하는 카운터.

**레이블:**
- `channel`: 채널 유형 (`sms`, `email` 또는 `dingtalk`)
- `purpose`: 챌린지 목적 (예: `login`, `reset`, `bind`)
- `result`: 작업 결과 (`success` 또는 `failed`)

**예:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### 전송 메트릭

#### `herald_otp_sends_total`

프로바이더를 통해 전송된 OTP의 총 수를 추적하는 카운터.

**레이블:**
- `channel`: 채널 유형 (`sms`, `email` 또는 `dingtalk`)
- `provider`: 프로바이더 이름 (예: `smtp`, `aliyun`, `placeholder`)
- `result`: 전송 작업 결과 (`success` 또는 `failed`)

**예:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

OTP 전송 작업의 지속 시간을 추적하는 히스토그램.

**레이블:**
- `provider`: 프로바이더 이름

**버킷:** 기본 Prometheus 버킷 (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**예:**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### 검증 메트릭

#### `herald_otp_verifications_total`

OTP 검증의 총 수를 추적하는 카운터.

**레이블:**
- `result`: 검증 결과 (`success` 또는 `failed`)
- `reason`: 실패 이유 (예: `expired`, `invalid`, `locked`, `too_many_attempts`)

**예:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### 속도 제한 메트릭

#### `herald_rate_limit_hits_total`

속도 제한 히트의 총 수를 추적하는 카운터.

**레이블:**
- `scope`: 속도 제한 범위 (`user`, `ip`, `destination`, `resend_cooldown`)

**예:**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Redis 메트릭

#### `herald_redis_latency_seconds`

Redis 작업 지연 시간을 추적하는 히스토그램.

**레이블:**
- `operation`: Redis 작업 유형 (`get`, `set`, `del`, `exists`)

**버킷:** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**예:**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Prometheus 구성

### 기본 스크랩 구성

`prometheus.yml`에 다음을 추가합니다:

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
```

### 서비스 디스커버리

Kubernetes 배포의 경우 서비스 디스커버리를 사용합니다:

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

## 모니터링할 주요 메트릭

### 비즈니스 메트릭

1. **챌린지 생성 속도**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **챌린지 성공률**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **검증 성공률**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **프로바이더별 전송 성공률**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### 성능 메트릭

1. **평균 전송 지속 시간**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **P95 전송 지속 시간**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Redis P99 지연 시간**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### 오류 메트릭

1. **속도 제한 히트율**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **검증 실패 이유**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **전송 실패율**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## 알림 규칙

### 알림 규칙 예제

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # 높은 실패율
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald 검증 실패율이 높습니다"
          description: "검증 실패율은 {{ $value | humanizePercentage }}입니다"

      # 높은 전송 실패율
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald 전송 실패율이 높습니다"
          description: "전송 실패율은 {{ $value | humanizePercentage }}입니다"

      # 높은 Redis 지연 시간
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald Redis 지연 시간이 높습니다"
          description: "P99 Redis 지연 시간은 {{ $value }}s입니다"

      # 높은 속도 제한 히트
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Herald 속도 제한 히트가 높습니다"
          description: "속도 제한 히트율은 {{ $value }} 히트/초입니다"
```

## Grafana 대시보드

### 권장 대시보드 패널

1. **개요 패널**
   - 챌린지 생성 속도
   - 검증 성공률
   - 전송 성공률
   - 현재 활성 챌린지 수 (추적되는 경우)

2. **성능 패널**
   - 전송 지속 시간 (P50, P95, P99)
   - Redis 지연 시간 (P50, P95, P99)
   - 요청 속도

3. **오류 패널**
   - 검증 실패 이유 분석
   - 프로바이더별 전송 실패율
   - 범위별 속도 제한 히트

4. **프로바이더 패널**
   - 프로바이더별 전송 성공률
   - 프로바이더별 전송 지속 시간
   - 프로바이더별 전송량

## OpenTelemetry 지원

**참고**: 분산 추적을 위한 OpenTelemetry 지원은 계획되어 있지만 아직 구현되지 않았습니다. 여기에는 다음이 포함됩니다:
- `traceparent` / `tracestate` 헤더 전파
- 핵심 스팬: `otp.challenge.create`, `otp.provider.send`, `otp.verify`
- 스팬 태그: `channel`, `purpose`, `provider`, `result`, `reason`

구현 일정은 프로젝트 로드맵을 참조하세요.

## 모범 사례

1. **주요 비즈니스 메트릭 모니터링**: 챌린지 생성, 검증 성공률 및 전송 성공률 추적
2. **알림 설정**: 높은 실패율 및 성능 저하에 대한 알림 구성
3. **프로바이더 성능 추적**: 프로바이더별 전송 지속 시간 및 성공률 모니터링
4. **Redis 상태 모니터링**: Redis 지연 시간 및 연결 문제 추적
5. **속도 제한 모니터링**: 속도 제한 히트를 모니터링하여 사용 패턴 및 잠재적 남용 파악

## 관련 문서

- [API 문서](API.md) - API 엔드포인트 세부 정보
- [배포 가이드](DEPLOYMENT.md) - 배포 및 구성
- [문제 해결 가이드](TROUBLESHOOTING.md) - 일반적인 문제 및 해결 방법
