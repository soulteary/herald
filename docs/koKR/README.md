# 문서 인덱스

Herald OTP 및 인증 코드 서비스 문서에 오신 것을 환영합니다.

## 🌐 다국어 문서

- [English](../enUS/README.md) | [中文](../zhCN/README.md) | [Français](../frFR/README.md) | [Italiano](../itIT/README.md) | [日本語](../jaJP/README.md) | [Deutsch](../deDE/README.md) | [한국어](README.md)

## 📚 문서 목록

### 핵심 문서

- **[README.md](../../README.koKR.md)** - 프로젝트 개요 및 빠른 시작 가이드

### 상세 문서

- **[API.md](API.md)** - 완전한 API 엔드포인트 문서
  - 인증 방법
  - 헬스 체크 엔드포인트
  - 챌린지 생성 및 검증
  - 속도 제한
  - 오류 코드 및 응답

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - 배포 가이드
  - Docker Compose 배포
  - 수동 배포
  - 구성 옵션
  - Stargate 통합
  - 보안 모범 사례

## 🚀 빠른 탐색

### 시작하기

1. [README.koKR.md](../../README.koKR.md)를 읽어 프로젝트를 이해하세요
2. [빠른 시작](../../README.koKR.md#빠른-시작) 섹션을 확인하세요
3. [구성](../../README.koKR.md#구성)을 참조하여 서비스를 구성하세요

### 개발자

1. [API.md](API.md)를 확인하여 API 인터페이스를 이해하세요
2. [DEPLOYMENT.md](DEPLOYMENT.md)를 검토하여 배포 옵션을 확인하세요

### 운영

1. [DEPLOYMENT.md](DEPLOYMENT.md)를 읽어 배포 방법을 이해하세요
2. [API.md](API.md)를 확인하여 API 엔드포인트 세부 정보를 확인하세요
3. [보안](DEPLOYMENT.md#보안)을 참조하여 보안 모범 사례를 확인하세요

## 📖 문서 구조

```
herald/
├── README.md              # 프로젝트 주 문서 (영어)
├── README.koKR.md         # 프로젝트 주 문서 (한국어)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # 문서 인덱스 (영어)
│   │   ├── API.md          # API 문서 (영어)
│   │   └── DEPLOYMENT.md   # 배포 가이드 (영어)
│   └── koKR/
│       ├── README.md       # 문서 인덱스 (한국어, 이 파일)
│       ├── API.md          # API 문서 (한국어)
│       └── DEPLOYMENT.md   # 배포 가이드 (한국어)
└── ...
```

## 🔍 주제별 검색

### API 관련

- API 엔드포인트 목록: [API.md](API.md)
- 인증 방법: [API.md#인증](API.md#인증)
- 오류 처리: [API.md#오류-코드](API.md#오류-코드)
- 속도 제한: [API.md#속도-제한](API.md#속도-제한)

### 배포 관련

- Docker 배포: [DEPLOYMENT.md#빠른-시작](DEPLOYMENT.md#빠른-시작)
- 구성 옵션: [DEPLOYMENT.md#구성](DEPLOYMENT.md#구성)
- Stargate 통합: [DEPLOYMENT.md#stargate-통합](DEPLOYMENT.md#stargate-통합)
- 보안: [DEPLOYMENT.md#보안](DEPLOYMENT.md#보안)

## 💡 사용 권장 사항

1. **처음 사용하는 사용자**: [README.koKR.md](../../README.koKR.md)로 시작하여 빠른 시작 가이드를 따르세요
2. **서비스 구성**: [DEPLOYMENT.md](DEPLOYMENT.md)를 참조하여 모든 구성 옵션을 이해하세요
3. **서비스 통합**: [DEPLOYMENT.md](DEPLOYMENT.md)의 통합 섹션을 확인하세요
4. **API 통합**: [API.md](API.md)를 읽어 API 인터페이스를 이해하세요

## 📝 문서 업데이트

문서는 프로젝트가 발전함에 따라 지속적으로 업데이트됩니다. 오류를 발견하거나 추가가 필요한 경우 Issue 또는 Pull Request를 제출해 주세요.

## 🤝 기여

문서 개선을 환영합니다:

1. 오류나 개선이 필요한 영역을 찾으세요
2. 문제를 설명하는 Issue를 제출하세요
3. 또는 직접 Pull Request를 제출하세요
