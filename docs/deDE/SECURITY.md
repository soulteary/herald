# Sicherheitsdokumentation

> 🌐 **Language / 语言**: [English](../enUS/SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](SECURITY.md) | [한국어](../koKR/SECURITY.md)

Dieses Dokument erläutert die Sicherheitsfunktionen von Herald, die Sicherheitskonfiguration und bewährte Praktiken.

> ⚠️ **Hinweis**: Diese Dokumentation wird derzeit übersetzt. Für die vollständige Version konsultieren Sie die [englische Version](../enUS/SECURITY.md).

## Implementierte Sicherheitsfunktionen

1. **Challenge-basierte Verifizierung**: Verwendet das Challenge-Verify-Modell, um Replay-Angriffe zu verhindern und die einmalige Verwendung von Verifizierungscodes sicherzustellen
2. **Sichere Codespeicherung**: Verifizierungscodes werden als Argon2-Hashes gespeichert, niemals im Klartext
3. **Mehrdimensionale Rate-Limiting**: Rate-Limiting nach user_id, Ziel (E-Mail/Telefon) und IP-Adresse zur Verhinderung von Missbrauch
4. **Service-Authentifizierung**: Unterstützt mTLS, HMAC-Signatur und API-Schlüssel-Authentifizierung für die Kommunikation zwischen Diensten
5. **Idempotenz-Schutz**: Verhindert doppelte Challenge-Erstellung und doppelte Code-Übermittlung mit Idempotenz-Schlüsseln
6. **Challenge-Ablauf**: Automatischer Ablauf von Challenges mit konfigurierbarem TTL
7. **Versuchsbeschränkung**: Maximale Versuchsgrenzen pro Challenge zur Verhinderung von Brute-Force-Angriffen
8. **Erneutes Senden Cooldown**: Verhindert schnelles erneutes Senden von Verifizierungscodes
9. **Audit-Protokollierung**: Vollständige Audit-Spur für alle Vorgänge, einschließlich Sendungen, Verifizierungen und Fehlern
10. **Provider-Sicherheit**: Sichere Kommunikation mit E-Mail- und SMS-Providern

Weitere Details finden Sie in der [englischen Version](../enUS/SECURITY.md).

## Meldung von Sicherheitslücken

Wenn Sie eine Sicherheitslücke entdecken, melden Sie diese bitte über:

1. **GitHub Security Advisory** (Bevorzugt)
   - Gehen Sie zur Registerkarte [Security](https://github.com/soulteary/herald/security) im Repository
   - Klicken Sie auf "Report a vulnerability"
   - Füllen Sie das Sicherheitsberatungsformular aus

2. **E-Mail** (Wenn GitHub Security Advisory nicht verfügbar ist)
   - Senden Sie eine E-Mail an die Projektbetreuer
   - Fügen Sie eine detaillierte Beschreibung der Sicherheitslücke bei

**Bitte melden Sie Sicherheitslücken nicht über öffentliche GitHub Issues.**
