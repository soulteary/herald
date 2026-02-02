# Herald 배포 가이드

## 빠른 시작

### Docker Compose 사용

```bash
cd herald
docker-compose up -d
```

### 수동 배포

```bash
# 빌드
go build -o herald main.go

# 실행
./herald
```

## 구성

### 환경 변수

| 변수 | 설명 | 기본값 | 필수 |
|------|------|-------|------|
| `PORT` | 서버 포트(선행 콜론 포함 또는 제외, 예: `8082` 또는 `:8082`) | `:8082` | 아니오 |
| `REDIS_ADDR` | Redis 주소 | `localhost:6379` | 아니오 |
| `REDIS_PASSWORD` | Redis 비밀번호 | `` | 아니오 |
| `REDIS_DB` | Redis 데이터베이스 | `0` | 아니오 |
| `API_KEY` | 인증용 API 키 | `` | 권장 |
| `HMAC_SECRET` | 보안 인증용 HMAC 비밀 키 | `` | 선택 사항 |
| `LOG_LEVEL` | 로그 수준 | `info` | 아니오 |
| `CHALLENGE_EXPIRY` | 챌린지 만료 시간 | `5m` | 아니오 |
| `MAX_ATTEMPTS` | 최대 인증 시도 횟수 | `5` | 아니오 |
| `RESEND_COOLDOWN` | 재전송 대기 시간 | `60s` | 아니오 |
| `CODE_LENGTH` | 인증 코드 길이 | `6` | 아니오 |
| `RATE_LIMIT_PER_USER` | 사용자당/시간 속도 제한 | `10` | 아니오 |
| `RATE_LIMIT_PER_IP` | IP당/분 속도 제한 | `5` | 아니오 |
| `RATE_LIMIT_PER_DESTINATION` | 대상당/시간 속도 제한 | `10` | 아니오 |
| `LOCKOUT_DURATION` | 최대 시도 횟수 후 사용자 잠금 기간 | `10m` | 아니오 |
| `SERVICE_NAME` | HMAC 인증용 서비스 식별자 | `herald` | 아니오 |
| `SMTP_HOST` | SMTP 서버 호스트 | `` | 이메일용 |
| `SMTP_PORT` | SMTP 서버 포트 | `587` | 이메일용 |
| `SMTP_USER` | SMTP 사용자 이름 | `` | 이메일용 |
| `SMTP_PASSWORD` | SMTP 비밀번호 | `` | 이메일용 |
| `SMTP_FROM` | SMTP 발신자 주소 | `` | 이메일용 |
| `SMS_PROVIDER` | SMS 제공자 | `` | SMS용 |
| `ALIYUN_ACCESS_KEY` | Aliyun 액세스 키 | `` | Aliyun SMS용 |
| `ALIYUN_SECRET_KEY` | Aliyun 비밀 키 | `` | Aliyun SMS용 |
| `ALIYUN_SIGN_NAME` | Aliyun SMS 서명 이름 | `` | Aliyun SMS용 |
| `ALIYUN_TEMPLATE_CODE` | Aliyun SMS 템플릿 코드 | `` | Aliyun SMS용 |
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) 기본 URL (예: `http://herald-dingtalk:8083`) | `` | DingTalk 채널용 |
| `HERALD_DINGTALK_API_KEY` | 선택적 API 키; herald-dingtalk의 `API_KEY`와 일치해야 함 (설정 시) | `` | 아니오 |

### DingTalk 채널 (herald-dingtalk)

`channel`이 `dingtalk`일 때 Herald는 직접 메시지를 보내지 않고 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)로 HTTP를 통해 전달합니다. 모든 DingTalk 자격 증명과 비즈니스 로직은 herald-dingtalk에 있으며 Herald는 DingTalk 자격 증명을 저장하지 않습니다. `HERALD_DINGTALK_API_URL`을 herald-dingtalk 서비스의 기본 URL로 설정하세요. herald-dingtalk에서 `API_KEY`를 설정한 경우 `HERALD_DINGTALK_API_KEY`를 동일한 값으로 설정하세요.

## 다른 서비스와의 통합 (선택사항)

Herald는 독립적으로 작동하도록 설계되었으며 필요에 따라 다른 서비스와 통합할 수 있습니다. Herald를 다른 인증 서비스나 게이트웨이 서비스와 통합하려면 다음을 구성할 수 있습니다:

**통합 구성 예:**
```bash
# Herald에 액세스할 수 있는 서비스 URL
export HERALD_URL=http://herald:8082

# 서비스 간 인증을 위한 API 키
export HERALD_API_KEY=your-secret-key

# Herald 통합 활성화 (서비스가 지원하는 경우)
export HERALD_ENABLED=true
```

**참고**: Herald는 외부 서비스 종속성 없이 독립적으로 사용할 수 있습니다. 다른 서비스와의 통합은 선택사항이며 특정 사용 사례에 따라 다릅니다.

## 보안

- 프로덕션 환경에서 HMAC 인증 사용
- 강력한 API 키 설정
- 프로덕션 환경에서 TLS/HTTPS 사용
- 속도 제한을 적절히 구성
- Redis의 의심스러운 활동 모니터링
