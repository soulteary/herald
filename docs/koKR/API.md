# Herald API 문서

Herald는 SMS 및 이메일을 통해 인증 코드를 보내는 인증 코드 및 OTP 서비스로, 내장된 속도 제한 및 보안 제어를 제공합니다.

## 기본 URL

```
http://localhost:8082
```

## 인증

Herald는 다음 우선순위 순서로 세 가지 인증 방법을 지원합니다:

1. **mTLS** (가장 안전): 클라이언트 인증서 검증을 통한 상호 TLS (최고 우선순위)
2. **HMAC 서명** (보안): `X-Signature`, `X-Timestamp` 및 `X-Service` 헤더 설정
3. **API 키** (간단): `X-API-Key` 헤더 설정 (최저 우선순위)

### mTLS 인증

검증된 클라이언트 인증서로 HTTPS를 사용하는 경우 Herald는 자동으로 mTLS를 통해 요청을 인증합니다. 이것은 가장 안전한 방법이며 다른 인증 방법보다 우선합니다.

### HMAC 서명

HMAC 서명은 다음과 같이 계산됩니다:
```
HMAC-SHA256(timestamp:service:body, secret)
```

여기서:
- `timestamp`: Unix 타임스탬프(초)
- `service`: 서비스 식별자(예: "my-service", "api-gateway")
- `body`: 요청 본문(JSON 문자열)
- `secret`: HMAC 비밀 키

**참고**: 재생 공격을 방지하기 위해 타임스탬프는 서버 시간의 5분(300초) 이내여야 합니다. 타임스탬프 창은 구성 가능하지만 기본값은 5분입니다.

**참고**: 현재 키 순환을 위한 `X-Key-Id` 헤더는 지원되지 않습니다. 이 기능은 향후 버전에서 계획되어 있습니다.

## 엔드포인트

### 헬스 체크

**GET /healthz**

서비스 상태를 확인합니다.

**응답:**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### 챌린지 생성

**POST /v1/otp/challenges**

새로운 인증 챌린지를 생성하고 인증 코드를 보냅니다.

**요청:**
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

**응답:**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

**오류 응답:**

모든 오류 응답은 다음 형식을 따릅니다:
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "선택적 오류 메시지"
}
```

가능한 오류 코드:
- `invalid_request`: 요청 본문 구문 분석 실패
- `user_id_required`: 필수 필드 `user_id` 누락
- `invalid_channel`: 잘못된 채널 유형("sms" 또는 "email"이어야 함)
- `destination_required`: 필수 필드 `destination` 누락
- `rate_limit_exceeded`: 속도 제한 초과
- `resend_cooldown`: 재전송 대기 시간이 만료되지 않음
- `user_locked`: 사용자가 일시적으로 잠겨 있음
- `internal_error`: 내부 서버 오류

HTTP 상태 코드:
- `400 Bad Request`: 잘못된 요청 매개변수
- `401 Unauthorized`: 인증 실패
- `403 Forbidden`: 사용자 잠금
- `429 Too Many Requests`: 속도 제한 초과
- `500 Internal Server Error`: 내부 서버 오류

### 챌린지 검증

**POST /v1/otp/verifications**

챌린지 코드를 검증합니다.

**요청:**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**응답(성공):**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**응답(실패):**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**오류 응답:**

가능한 오류 코드:
- `invalid_request`: 요청 본문 구문 분석 실패
- `challenge_id_required`: 필수 필드 `challenge_id` 누락
- `code_required`: 필수 필드 `code` 누락
- `invalid_code_format`: 인증 코드 형식이 잘못됨
- `expired`: 챌린지가 만료됨
- `invalid`: 잘못된 인증 코드
- `locked`: 시도 횟수가 너무 많아 챌린지가 잠김
- `verification_failed`: 일반 인증 실패
- `internal_error`: 내부 서버 오류

HTTP 상태 코드:
- `400 Bad Request`: 잘못된 요청 매개변수
- `401 Unauthorized`: 검증 실패
- `403 Forbidden`: 사용자 잠금
- `500 Internal Server Error`: 내부 서버 오류

### 챌린지 취소

**POST /v1/otp/challenges/{id}/revoke**

챌린지를 취소합니다(선택 사항).

**응답(성공):**
```json
{
  "ok": true
}
```

**응답(실패):**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**오류 응답:**

가능한 오류 코드:
- `challenge_id_required`: URL 매개변수에 챌린지 ID 누락
- `internal_error`: 내부 서버 오류

HTTP 상태 코드:
- `400 Bad Request`: 잘못된 요청
- `500 Internal Server Error`: 내부 서버 오류

## 속도 제한

Herald는 다차원 속도 제한을 구현합니다:

- **사용자당**: 시간당 10개 요청(설정 가능)
- **IP당**: 분당 5개 요청(설정 가능)
- **대상당**: 시간당 10개 요청(설정 가능)
- **재전송 대기 시간**: 재전송 사이 60초

## 오류 코드

이 섹션에서는 API가 반환할 수 있는 모든 오류 코드를 나열합니다.

### 요청 유효성 검사 오류
- `invalid_request`: 요청 본문 구문 분석 실패 또는 잘못된 JSON
- `user_id_required`: 필수 필드 `user_id` 누락
- `invalid_channel`: 잘못된 채널 유형("sms" 또는 "email"이어야 함)
- `destination_required`: 필수 필드 `destination` 누락
- `challenge_id_required`: 필수 필드 `challenge_id` 누락
- `code_required`: 필수 필드 `code` 누락
- `invalid_code_format`: 인증 코드 형식이 잘못됨

### 인증 오류
- `authentication_required`: 유효한 인증이 제공되지 않음
- `invalid_timestamp`: 잘못된 타임스탬프 형식
- `timestamp_expired`: 타임스탬프가 허용된 창(5분) 밖에 있음
- `invalid_signature`: HMAC 서명 검증 실패

### 챌린지 오류
- `expired`: 챌린지가 만료됨
- `invalid`: 잘못된 인증 코드
- `locked`: 시도 횟수가 너무 많아 챌린지가 잠김
- `too_many_attempts`: 실패한 시도 횟수가 너무 많음(`locked`에 포함될 수 있음)
- `verification_failed`: 일반 인증 실패

### 속도 제한 오류
- `rate_limit_exceeded`: 속도 제한 초과
- `resend_cooldown`: 재전송 대기 시간이 만료되지 않음

### 사용자 상태 오류
- `user_locked`: 사용자가 일시적으로 잠겨 있음

### 시스템 오류
- `internal_error`: 내부 서버 오류
