# Herald - OTP 및 인증 코드 서비스

> **📧 안전한 인증을 위한 게이트웨이**

## 🌐 다국어 문서

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald는 이메일을 통해 인증 코드(OTP)를 보내는 프로덕션 준비가 된 경량 서비스입니다(SMS 지원은 현재 개발 중). 내장된 속도 제한, 보안 제어 및 감사 로깅이 포함되어 있습니다.

## 기능

- 🚀 **고성능** : Go 및 Fiber로 구축
- 🔒 **보안** : 해시 저장소를 사용한 챌린지 기반 인증
- 📊 **속도 제한** : 다차원 속도 제한(사용자별, IP별, 대상별)
- 📝 **감사 로깅** : 모든 작업에 대한 완전한 감사 추적
- 🔌 **플러그 가능한 제공자** : 이메일 제공자 지원(SMS 제공자는 자리 표시자 구현이며 아직 완전히 작동하지 않음)
- ⚡ **Redis 백엔드** : Redis를 사용한 빠르고 분산된 스토리지

## 빠른 시작

```bash
# Docker Compose로 실행
docker-compose up -d

# 또는 직접 실행
go run main.go
```

## 구성

환경 변수 설정 :

- `PORT` : 서버 포트(기본값 : `:8082`)
- `REDIS_ADDR` : Redis 주소(기본값 : `localhost:6379`)
- `REDIS_PASSWORD` : Redis 비밀번호(선택 사항)
- `REDIS_DB` : Redis 데이터베이스 번호(기본값 : `0`)
- `API_KEY` : 서비스 간 인증을 위한 API 키
- `LOG_LEVEL` : 로그 수준(기본값 : `info`)

전체 구성 옵션은 [DEPLOYMENT.md](docs/koKR/DEPLOYMENT.md)를 참조하세요.

## API 문서

자세한 API 문서는 [API.md](docs/koKR/API.md)를 참조하세요.
