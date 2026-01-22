# Documentazione di Sicurezza

> 🌐 **Language / 语言**: [English](../enUS/SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](../koKR/SECURITY.md)

Questo documento spiega le funzionalità di sicurezza di Herald, la configurazione di sicurezza e le migliori pratiche.

> ⚠️ **Nota**: Questa documentazione è in fase di traduzione. Per la versione completa, consulta la [versione inglese](../enUS/SECURITY.md).

## Funzionalità di Sicurezza Implementate

1. **Verifica basata su Challenge**: Utilizza il modello challenge-verify per prevenire attacchi di replay e garantire l'uso una tantum dei codici di verifica
2. **Archiviazione sicura dei codici**: I codici di verifica sono archiviati come hash Argon2, mai in testo normale
3. **Limitazione della velocità multidimensionale**: Limitazione della velocità per user_id, destinazione (email/telefono) e indirizzo IP per prevenire abusi
4. **Autenticazione del servizio**: Supporta mTLS, firma HMAC e autenticazione tramite chiave API per la comunicazione inter-servizio
5. **Protezione dell'idempotenza**: Previene la creazione duplicata di challenge e l'invio duplicato di codici utilizzando chiavi di idempotenza
6. **Scadenza dei challenge**: Scadenza automatica dei challenge con TTL configurabile
7. **Limitazione dei tentativi**: Limiti massimi di tentativi per challenge per prevenire attacchi di forza bruta
8. **Cooldown di reinvio**: Previene il reinvio rapido dei codici di verifica
9. **Registrazione di audit**: Traccia di audit completa per tutte le operazioni, inclusi invii, verifiche e fallimenti
10. **Sicurezza del provider**: Comunicazione sicura con provider email e SMS

Per maggiori dettagli, consulta la [versione inglese](../enUS/SECURITY.md).

## Segnalazione di Vulnerabilità

Se scopri una vulnerabilità di sicurezza, segnalala tramite:

1. **GitHub Security Advisory** (Preferito)
   - Vai alla scheda [Security](https://github.com/soulteary/herald/security) nel repository
   - Clicca su "Report a vulnerability"
   - Compila il modulo di consulenza sulla sicurezza

2. **Email** (Se GitHub Security Advisory non è disponibile)
   - Invia un'email ai maintainer del progetto
   - Includi una descrizione dettagliata della vulnerabilità

**Si prega di non segnalare vulnerabilità di sicurezza tramite problemi GitHub pubblici.**
