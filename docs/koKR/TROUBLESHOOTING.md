# Herald 문제 해결 가이드

이 가이드는 Herald OTP 및 인증 코드 서비스의 일반적인 문제를 진단하고 해결하는 데 도움이 됩니다.

## 목차

- [인증 코드를 받지 못함](#인증-코드를-받지-못함)
- [인증 코드 오류](#인증-코드-오류)
- [401 인증되지 않음 오류](#401-인증되지-않음-오류)
- [속도 제한 문제](#속도-제한-문제)
- [Redis 연결 문제](#redis-연결-문제)
- [프로바이더 전송 실패](#프로바이더-전송-실패)
- [성능 문제](#성능-문제)

## 인증 코드를 받지 못함

### 증상
- 사용자가 SMS 또는 이메일을 통해 인증 코드를 받지 못했다고 보고
- 챌린지 생성은 성공하지만 코드가 전달되지 않음

### 진단 단계

1. **프로바이더 연결 확인**
   ```bash
   # Herald 로그에서 프로바이더 오류 확인
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **프로바이더 구성 확인**
   - SMTP 설정 확인 (이메일용): `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - SMS 프로바이더 설정 확인: `SMS_PROVIDER`, `ALIYUN_ACCESS_KEY` 등
   - 프로바이더 자격 증명이 올바른지 확인

3. **Prometheus 메트릭 확인**
   ```promql
   # 전송 실패율 확인
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # 프로바이더별 전송 성공 확인
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **감사 로그 확인**
   - `send_failed` 이벤트에 대한 감사 로그 검토
   - 프로바이더 오류 코드 및 메시지 확인
   - 대상 주소가 유효한지 확인

5. **프로바이더 직접 테스트**
   - SMTP 연결을 수동으로 테스트
   - SMS 프로바이더 API를 직접 테스트
   - 프로바이더 엔드포인트에 대한 네트워크 연결 확인

### 해결 방법

- **프로바이더 구성 문제**: 프로바이더 자격 증명 및 설정 업데이트
- **네트워크 문제**: 방화벽 규칙 및 네트워크 연결 확인
- **프로바이더 속도 제한**: 프로바이더에 초과되는 속도 제한이 있는지 확인
- **잘못된 대상**: 이메일 주소 및 전화번호가 유효한지 확인

## 인증 코드 오류

### 증상
- 사용자가 "잘못된 코드" 오류를 보고
- 올바른 코드로도 검증이 실패함
- 챌린지가 만료되었거나 잠겨 있는 것으로 보임

### 진단 단계

1. **챌린지 상태 확인**
   ```bash
   # Redis에서 챌린지 데이터 확인
   redis-cli GET "otp:ch:{challenge_id}"
   ```

2. **챌린지 만료 확인**
   - `CHALLENGE_EXPIRY` 구성 확인 (기본값: 5분)
   - 시스템 시간이 동기화되었는지 확인 (NTP)
   - 챌린지가 만료되었는지 확인

3. **시도 횟수 확인**
   - `MAX_ATTEMPTS` 구성 확인 (기본값: 5)
   - 너무 많은 시도로 인해 챌린지가 잠겨 있는지 확인
   - 시도 기록에 대한 감사 로그 검토

4. **Prometheus 메트릭 확인**
   ```promql
   # 검증 실패 이유 확인
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # 잠긴 챌린지 확인
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
   ```

5. **감사 로그 검토**
   - 검증 시도 및 결과 확인
   - 코드 형식이 예상 형식과 일치하는지 확인
   - 타이밍 문제 확인

### 해결 방법

- **만료된 챌린지**: 사용자는 새로운 인증 코드를 요청해야 함
- **너무 많은 시도**: 챌린지가 잠겨 있으므로 잠금 기간을 기다리거나 새 챌린지를 요청
- **코드 형식 불일치**: 코드 길이가 `CODE_LENGTH` 구성과 일치하는지 확인
- **시간 동기화**: 시스템 시계가 동기화되었는지 확인

## 401 인증되지 않음 오류

### 증상
- API 요청이 401 인증되지 않음 반환
- 로그에 인증 실패 기록
- 서비스 간 통신 실패

### 진단 단계

1. **인증 방법 확인**
   - 사용 중인 인증 방법 확인 (mTLS, HMAC, API 키)
   - 인증 헤더가 있는지 확인

2. **HMAC 인증의 경우**
   ```bash
   # HMAC_SECRET이 구성되었는지 확인
   echo $HMAC_SECRET
   
   # 타임스탬프가 5분 창 내에 있는지 확인
   # 서명 계산이 Herald의 구현과 일치하는지 확인
   ```

3. **API 키 인증의 경우**
   ```bash
   # API_KEY가 설정되었는지 확인
   echo $API_KEY
   
   # X-API-Key 헤더가 구성된 키와 일치하는지 확인
   ```

4. **mTLS 인증의 경우**
   - 클라이언트 인증서가 유효한지 확인
   - 인증서 체인이 신뢰되는지 확인
   - TLS 연결이 설정되었는지 확인

5. **Herald 로그 확인**
   ```bash
   # 인증 오류 확인
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### 해결 방법

- **누락된 자격 증명**: `API_KEY` 또는 `HMAC_SECRET` 환경 변수 설정
- **잘못된 서명**: HMAC 서명 계산이 Herald의 구현과 일치하는지 확인
- **만료된 타임스탬프**: 클라이언트와 서버 시계가 동기화되었는지 확인 (5분 이내)
- **인증서 문제**: mTLS 인증서가 유효하고 신뢰되는지 확인

## 속도 제한 문제

### 증상
- 사용자가 "속도 제한 초과" 오류를 보고
- 합법적인 요청이 차단됨
- 높은 속도 제한 히트 메트릭

### 진단 단계

1. **속도 제한 구성 확인**
   ```bash
   # 속도 제한 설정 확인
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **Prometheus 메트릭 확인**
   ```promql
   # 범위별 속도 제한 히트 확인
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # 속도 제한 히트율 확인
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Redis 키 검토**
   ```bash
   # 속도 제한 키 확인
   redis-cli KEYS "otp:rate:*"
   
   # 특정 속도 제한 키 확인
   redis-cli GET "otp:rate:user:{user_id}"
   ```

4. **사용 패턴 분석**
   - 합법적인 사용자가 제한에 도달하는지 확인
   - 잠재적 남용 또는 봇 활동 식별
   - 챌린지 생성 패턴 검토

### 해결 방법

- **속도 제한 조정**: 합법적인 사용자가 영향을 받는 경우 제한 증가
- **사용자 교육**: 사용자에게 속도 제한 및 쿨다운 기간에 대해 알림
- **남용 방지**: 의심되는 남용에 대해 추가 보안 조치 구현
- **재전송 쿨다운**: 사용자는 `RESEND_COOLDOWN` 기간을 기다려야 함 (기본값: 60초)

## Redis 연결 문제

### 증상
- 서비스가 시작되지 않음
- 헬스 체크가 비정상 반환
- 챌린지 생성/검증 실패
- 높은 Redis 지연 시간 메트릭

### 진단 단계

1. **Redis 연결성 확인**
   ```bash
   # Redis 연결 테스트
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **구성 확인**
   ```bash
   # Redis 구성 확인
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   echo $REDIS_DB
   ```

3. **Redis 상태 확인**
   ```bash
   # Redis 정보 확인
   redis-cli INFO
   
   # 메모리 사용량 확인
   redis-cli INFO memory
   ```

4. **Prometheus 메트릭 확인**
   ```promql
   # Redis 지연 시간 확인
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # Redis 작업 오류 확인
   rate(herald_redis_latency_seconds_count[5m])
   ```

5. **Herald 로그 검토**
   ```bash
   # Redis 오류 확인
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### 해결 방법

- **연결 문제**: `REDIS_ADDR`이 올바르고 Redis에 액세스 가능한지 확인
- **인증 문제**: `REDIS_PASSWORD`가 올바른지 확인
- **데이터베이스 선택**: `REDIS_DB`가 올바른지 확인 (기본값: 0)
- **성능 문제**: Redis 메모리 사용량을 확인하고 스케일링 고려
- **네트워크 문제**: 네트워크 연결 및 방화벽 규칙 확인

## 프로바이더 전송 실패

### 증상
- 메트릭에서 높은 전송 실패율
- 로그에 `send_failed` 오류 기록
- 프로바이더별 오류 메시지

### 진단 단계

1. **프로바이더 메트릭 확인**
   ```promql
   # 프로바이더별 전송 실패율 확인
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # 프로바이더별 전송 지속 시간 확인
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **프로바이더 로그 검토**
   - 프로바이더별 오류 메시지 확인
   - 프로바이더 오류에 대한 감사 로그 검토
   - 프로바이더 API 응답 코드 확인

3. **프로바이더 연결 테스트**
   - SMTP 연결을 수동으로 테스트
   - SMS 프로바이더 API를 직접 테스트
   - 프로바이더 자격 증명 확인

4. **프로바이더 상태 확인**
   - 프로바이더 상태 페이지 확인
   - 프로바이더 API가 작동하는지 확인
   - 프로바이더 속도 제한 확인

### 해결 방법

- **프로바이더 중단**: 프로바이더가 서비스를 복원할 때까지 대기
- **자격 증명 문제**: 프로바이더 자격 증명 업데이트
- **속도 제한**: 프로바이더 속도 제한이 초과되었는지 확인
- **구성 문제**: 프로바이더 구성이 올바른지 확인
- **네트워크 문제**: 프로바이더에 대한 네트워크 연결 확인

## 성능 문제

### 증상
- 느린 응답 시간
- 높은 지연 시간 메트릭
- 타임아웃 오류
- 높은 리소스 사용량

### 진단 단계

1. **응답 시간 확인**
   ```promql
   # 전송 지속 시간 확인
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # Redis 지연 시간 확인
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **리소스 사용량 확인**
   ```bash
   # CPU 및 메모리 확인
   top -p $(pgrep herald)
   
   # Goroutine 수 확인
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
   ```

3. **요청 패턴 검토**
   - 요청 속도 확인
   - 피크 사용 시간 식별
   - 트래픽 스파이크 확인

4. **Redis 성능 확인**
   ```bash
   # Redis 느린 로그 확인
   redis-cli SLOWLOG GET 10
   
   # Redis 메모리 확인
   redis-cli INFO memory
   ```

### 해결 방법

- **리소스 스케일링**: CPU/메모리 할당 증가
- **Redis 최적화**: Redis Cluster 사용 또는 쿼리 최적화
- **프로바이더 최적화**: 더 빠른 프로바이더 사용 또는 프로바이더 호출 최적화
- **캐싱**: 자주 액세스되는 데이터에 대한 캐싱 구현
- **로드 밸런싱**: 여러 인스턴스 간에 로드 분산

## 도움 받기

문제를 해결할 수 없는 경우:

1. **로그 확인**: 자세한 오류 메시지에 대해 Herald 로그 검토
2. **메트릭 확인**: 패턴에 대해 Prometheus 메트릭 검토
3. **문서 검토**: [API.md](API.md) 및 [DEPLOYMENT.md](DEPLOYMENT.md) 확인
4. **문제 보고**: 다음을 포함하는 문제 생성:
   - 오류 메시지 및 로그
   - 구성 (비밀 제외)
   - 재현 단계
   - 예상 동작과 실제 동작

## 관련 문서

- [API 문서](API.md) - API 엔드포인트 세부 정보
- [배포 가이드](DEPLOYMENT.md) - 배포 및 구성
- [모니터링 가이드](MONITORING.md) - 모니터링 및 메트릭
