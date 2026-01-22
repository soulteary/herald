# Herald - OTP 및 인증 코드 서비스

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![codecov](https://codecov.io/gh/soulteary/herald/branch/main/graph/badge.svg)](https://codecov.io/gh/soulteary/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/herald)](https://goreportcard.com/report/github.com/soulteary/herald)

> **📧 안전한 인증을 위한 게이트웨이**

## 🌐 다국어 문서

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald는 이메일과 SMS를 통해 인증 코드를 보낼 수 있는 프로덕션 준비가 된 독립형 OTP 및 인증 코드 서비스입니다. 내장된 속도 제한, 보안 제어 및 감사 로깅 기능을 제공합니다. Herald는 독립적으로 작동하도록 설계되었으며 필요에 따라 다른 서비스와 통합할 수 있습니다.

## 핵심 기능

- 🔒 **보안 설계**：Argon2 해시 스토리지를 사용한 챌린지 기반 인증, 여러 인증 방법(mTLS, HMAC, API Key)
- 📊 **내장 속도 제한**：다차원 속도 제한(사용자별, IP별, 대상별), 구성 가능한 임계값
- 📝 **완전한 감사 추적**：프로바이더 추적을 포함한 모든 작업의 완전한 감사 로깅
- 🔌 **플러그 가능한 프로바이더**：확장 가능한 이메일 및 SMS 프로바이더 아키텍처

## 빠른 시작

### Docker Compose 사용

가장 쉬운 방법은 Redis를 포함하는 Docker Compose를 사용하는 것입니다:

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

예상 응답:
```json
{
  "status": "ok",
  "service": "herald"
}
```

### API 테스트

테스트 챌린지 생성(인증 필요 - [API 문서](docs/koKR/API.md) 참조):

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

### 로그 보기

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### 수동 배포

수동 배포 및 고급 구성에 대해서는 [배포 가이드](docs/koKR/DEPLOYMENT.md)를 참조하세요.

## 기본 구성

Herald를 시작하려면 최소한의 구성이 필요합니다:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

속도 제한, 챌린지 만료 시간 및 프로바이더 설정을 포함한 전체 구성 옵션에 대해서는 [배포 가이드](docs/koKR/DEPLOYMENT.md#configuration)를 참조하세요.

## 문서

### 개발자용

- **[API 문서](docs/koKR/API.md)** - 인증 방법, 엔드포인트 및 오류 코드를 포함한 전체 API 참조
- **[배포 가이드](docs/koKR/DEPLOYMENT.md)** - 구성 옵션, Docker 배포 및 통합 예제

### 운영용

- **[모니터링 가이드](docs/koKR/MONITORING.md)** - Prometheus 메트릭, Grafana 대시보드 및 알림
- **[문제 해결 가이드](docs/koKR/TROUBLESHOOTING.md)** - 일반적인 문제, 진단 단계 및 해결 방법

### 문서 인덱스

모든 문서의 전체 개요는 [docs/koKR/README.md](docs/koKR/README.md)를 참조하세요.

## License

See [LICENSE](LICENSE) for details.
