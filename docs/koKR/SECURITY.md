# 보안 문서

> 🌐 **Language / 语言**: [English](../enUS/SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](SECURITY.md)

이 문서는 Herald의 보안 기능, 보안 구성 및 모범 사례를 설명합니다.

> ⚠️ **참고**: 이 문서는 현재 번역 중입니다. 전체 버전은 [영어 버전](../enUS/SECURITY.md)을 참조하세요.

## 구현된 보안 기능

1. **챌린지 기반 검증**: 재생 공격을 방지하고 검증 코드의 일회성 사용을 보장하기 위해 challenge-verify 모델 사용
2. **안전한 코드 저장**: 검증 코드는 Argon2 해시로 저장되며 평문으로 저장되지 않음
3. **다차원 속도 제한**: user_id, 대상(이메일/전화) 및 IP 주소별 속도 제한으로 남용 방지
4. **서비스 인증**: 서비스 간 통신을 위한 mTLS, HMAC 서명 및 API 키 인증 지원
5. **멱등성 보호**: 멱등 키를 사용하여 중복 챌린지 생성 및 코드 중복 전송 방지
6. **챌린지 만료**: 구성 가능한 TTL로 챌린지 자동 만료
7. **시도 제한**: 무차별 대입 공격을 방지하기 위한 챌린지당 최대 시도 제한
8. **재전송 쿨다운**: 검증 코드의 빠른 재전송 방지
9. **감사 로깅**: 전송, 검증 및 실패를 포함한 모든 작업에 대한 완전한 감사 추적
10. **프로바이더 보안**: 이메일 및 SMS 프로바이더와의 안전한 통신

자세한 내용은 [영어 버전](../enUS/SECURITY.md)을 참조하세요.

## 취약점 보고

보안 취약점을 발견한 경우 다음을 통해 보고하세요:

1. **GitHub Security Advisory** (권장)
   - 저장소의 [Security 탭](https://github.com/soulteary/herald/security)으로 이동
   - "Report a vulnerability" 클릭
   - 보안 자문 양식 작성

2. **이메일** (GitHub Security Advisory를 사용할 수 없는 경우)
   - 프로젝트 유지 관리자에게 이메일 보내기
   - 취약점에 대한 자세한 설명 포함

**공개 GitHub Issues를 통해 보안 취약점을 보고하지 마세요.**
