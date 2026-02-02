# Herald Fehlerbehebungsleitfaden

Dieser Leitfaden hilft Ihnen, häufige Probleme mit dem Herald OTP- und Verifizierungscode-Service zu diagnostizieren und zu lösen.

## Inhaltsverzeichnis

- [Verifizierungscode nicht erhalten](#verifizierungscode-nicht-erhalten)
- [Verifizierungscode-Fehler](#verifizierungscode-fehler)
- [401 Nicht autorisierte Fehler](#401-nicht-autorisierte-fehler)
- [Rate-Limiting-Probleme](#rate-limiting-probleme)
- [Redis-Verbindungsprobleme](#redis-verbindungsprobleme)
- [Provider-Send-Fehler](#provider-send-fehler)
- [Leistungsprobleme](#leistungsprobleme)

## Verifizierungscode nicht erhalten

### Symptome
- Benutzer meldet, dass kein Verifizierungscode per SMS oder E-Mail erhalten wurde
- Challenge-Erstellung erfolgreich, aber kein Code geliefert

### Diagnoseschritte

1. **Provider-Verbindung prüfen**
   ```bash
   # Herald-Protokolle auf Provider-Fehler prüfen
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **Provider-Konfiguration verifizieren**
   - SMTP-Einstellungen prüfen (bei eingebautem SMTP): `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - Bei E-Mail über herald-smtp: `HERALD_SMTP_API_URL` gesetzt und herald-smtp erreichbar; optional `HERALD_SMTP_API_KEY`. Bei nicht empfangenen E-Mails herald-smtp-Logs und SMTP-Konfiguration prüfen.
   - SMS-Provider-Einstellungen prüfen: `SMS_PROVIDER`, `ALIYUN_ACCESS_KEY`, etc.
   - Verifizieren, dass Provider-Anmeldedaten korrekt sind

3. **Prometheus-Metriken prüfen**
   ```promql
   # Send-Fehlerrate prüfen
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # Send-Erfolg nach Provider prüfen
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **Audit-Protokolle prüfen**
   - Audit-Protokolle auf `send_failed`-Ereignisse überprüfen
   - Provider-Fehlercodes und -Nachrichten prüfen
   - Verifizieren, dass Zieladressen gültig sind

5. **Provider direkt testen**
   - SMTP-Verbindung manuell testen
   - SMS-Provider-API direkt testen
   - Netzwerkverbindung zu Provider-Endpunkten verifizieren

### Lösungen

- **Provider-Konfigurationsprobleme**: Provider-Anmeldedaten und -Einstellungen aktualisieren
- **Netzwerkprobleme**: Firewall-Regeln und Netzwerkverbindung prüfen
- **Provider-Rate-Limits**: Prüfen, ob Provider-Rate-Limits überschritten werden
- **Ungültige Ziele**: E-Mail-Adressen und Telefonnummern verifizieren

## Verifizierungscode-Fehler

### Symptome
- Benutzer meldet "ungültiger Code"-Fehler
- Verifizierung schlägt fehl, auch mit richtigem Code
- Challenge erscheint abgelaufen oder gesperrt

### Diagnoseschritte

1. **Challenge-Status prüfen**
   ```bash
   # Redis auf Challenge-Daten prüfen
   redis-cli GET "otp:ch:{challenge_id}"
   ```

2. **Challenge-Ablauf verifizieren**
   - `CHALLENGE_EXPIRY`-Konfiguration prüfen (Standard: 5 Minuten)
   - Systemzeit synchronisiert verifizieren (NTP)
   - Prüfen, ob Challenge abgelaufen ist

3. **Versuchsanzahl prüfen**
   - `MAX_ATTEMPTS`-Konfiguration verifizieren (Standard: 5)
   - Prüfen, ob Challenge aufgrund zu vieler Versuche gesperrt ist
   - Audit-Protokolle auf Versuchsverlauf überprüfen

4. **Prometheus-Metriken prüfen**
   ```promql
   # Verifizierungs-Fehlergründe prüfen
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # Gesperrte Challenges prüfen
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
   ```

5. **Audit-Protokolle überprüfen**
   - Verifizierungsversuche und -ergebnisse prüfen
   - Code-Format mit erwartetem Format abgleichen
   - Auf Zeitprobleme prüfen

### Lösungen

- **Abgelaufene Challenge**: Benutzer muss neuen Verifizierungscode anfordern
- **Zu viele Versuche**: Challenge ist gesperrt, auf Sperrdauer warten oder neue Challenge anfordern
- **Code-Format stimmt nicht überein**: Code-Länge mit `CODE_LENGTH`-Konfiguration abgleichen
- **Zeitsynchronisation**: Sicherstellen, dass Systemuhren synchronisiert sind

## 401 Nicht autorisierte Fehler

### Symptome
- API-Anfragen geben 401 Nicht autorisiert zurück
- Authentifizierungsfehler in Protokollen
- Service-zu-Service-Kommunikation schlägt fehl

### Diagnoseschritte

1. **Authentifizierungsmethode prüfen**
   - Verifizieren, welche Authentifizierungsmethode verwendet wird (mTLS, HMAC, API-Schlüssel)
   - Prüfen, ob Authentifizierungs-Header vorhanden sind

2. **Für HMAC-Authentifizierung**
   ```bash
   # Verifizieren, dass HMAC_SECRET konfiguriert ist
   echo $HMAC_SECRET
   
   # Prüfen, ob Zeitstempel innerhalb des 5-Minuten-Fensters liegt
   # Verifizieren, dass Signaturberechnung mit Herald-Implementierung übereinstimmt
   ```

3. **Für API-Schlüssel-Authentifizierung**
   ```bash
   # Verifizieren, dass API_KEY gesetzt ist
   echo $API_KEY
   
   # Prüfen, ob X-API-Key-Header mit konfiguriertem Schlüssel übereinstimmt
   ```

4. **Für mTLS-Authentifizierung**
   - Verifizieren, dass Client-Zertifikat gültig ist
   - Zertifikatskette als vertrauenswürdig prüfen
   - TLS-Verbindung verifizieren

5. **Herald-Protokolle prüfen**
   ```bash
   # Auf Authentifizierungsfehler prüfen
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### Lösungen

- **Fehlende Anmeldedaten**: `API_KEY` oder `HMAC_SECRET` Umgebungsvariablen setzen
- **Ungültige Signatur**: Verifizieren, dass HMAC-Signaturberechnung mit Herald-Implementierung übereinstimmt
- **Zeitstempel abgelaufen**: Sicherstellen, dass Client- und Server-Uhren synchronisiert sind (innerhalb von 5 Minuten)
- **Zertifikatsprobleme**: Verifizieren, dass mTLS-Zertifikate gültig und vertrauenswürdig sind

## Rate-Limiting-Probleme

### Symptome
- Benutzer melden "Rate-Limit überschritten"-Fehler
- Legitime Anfragen werden blockiert
- Hohe Rate-Limit-Hit-Metriken

### Diagnoseschritte

1. **Rate-Limit-Konfiguration prüfen**
   ```bash
   # Rate-Limit-Einstellungen verifizieren
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **Prometheus-Metriken prüfen**
   ```promql
   # Rate-Limit-Hits nach Bereich prüfen
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # Rate-Limit-Hit-Rate prüfen
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Redis-Schlüssel überprüfen**
   ```bash
   # Rate-Limit-Schlüssel prüfen
   redis-cli KEYS "otp:rate:*"
   
   # Spezifischen Rate-Limit-Schlüssel prüfen
   redis-cli GET "otp:rate:user:{user_id}"
   ```

4. **Nutzungsmuster analysieren**
   - Prüfen, ob legitime Benutzer Limits erreichen
   - Potenziellen Missbrauch oder Bot-Aktivität identifizieren
   - Challenge-Erstellungsmuster überprüfen

### Lösungen

- **Rate-Limits anpassen**: Limits erhöhen, wenn legitime Benutzer betroffen sind
- **Benutzeraufklärung**: Benutzer über Rate-Limits und Abkühlungsperioden informieren
- **Missbrauchsprävention**: Zusätzliche Sicherheitsmaßnahmen für vermuteten Missbrauch implementieren
- **Erneutes Senden Abkühlung**: Benutzer müssen auf `RESEND_COOLDOWN`-Periode warten (Standard: 60 Sekunden)

## Redis-Verbindungsprobleme

### Symptome
- Service startet nicht
- Gesundheitsprüfung gibt ungesund zurück
- Challenge-Erstellung/Verifizierung schlägt fehl
- Hohe Redis-Latenz-Metriken

### Diagnoseschritte

1. **Redis-Verbindung prüfen**
   ```bash
   # Redis-Verbindung testen
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **Konfiguration verifizieren**
   ```bash
   # Redis-Konfiguration prüfen
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   echo $REDIS_DB
   ```

3. **Redis-Gesundheit prüfen**
   ```bash
   # Redis-Info prüfen
   redis-cli INFO
   
   # Speichernutzung prüfen
   redis-cli INFO memory
   ```

4. **Prometheus-Metriken prüfen**
   ```promql
   # Redis-Latenz prüfen
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # Redis-Operationsfehler prüfen
   rate(herald_redis_latency_seconds_count[5m])
   ```

5. **Herald-Protokolle überprüfen**
   ```bash
   # Auf Redis-Fehler prüfen
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### Lösungen

- **Verbindungsprobleme**: Verifizieren, dass `REDIS_ADDR` korrekt ist und Redis erreichbar ist
- **Authentifizierungsprobleme**: Prüfen, dass `REDIS_PASSWORD` korrekt ist
- **Datenbankauswahl**: Verifizieren, dass `REDIS_DB` korrekt ist (Standard: 0)
- **Leistungsprobleme**: Redis-Speichernutzung prüfen und Skalierung in Betracht ziehen
- **Netzwerkprobleme**: Netzwerkverbindung und Firewall-Regeln verifizieren

## Provider-Send-Fehler

### Symptome
- Hohe Send-Fehlerrate in Metriken
- `send_failed`-Fehler in Protokollen
- Provider-spezifische Fehlermeldungen

### Diagnoseschritte

1. **Provider-Metriken prüfen**
   ```promql
   # Send-Fehlerrate nach Provider prüfen
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # Send-Dauer nach Provider prüfen
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **Provider-Protokolle überprüfen**
   - Provider-spezifische Fehlermeldungen prüfen
   - Audit-Protokolle auf Provider-Fehler überprüfen
   - Provider-API-Antwortcodes prüfen

3. **Provider-Verbindung testen**
   - SMTP-Verbindung manuell testen
   - SMS-Provider-API direkt testen
   - Provider-Anmeldedaten verifizieren

4. **Provider-Status prüfen**
   - Provider-Statusseite prüfen
   - Verifizieren, dass Provider-API betriebsbereit ist
   - Auf Provider-Rate-Limits prüfen

### Lösungen

- **Provider-Ausfall**: Auf Wiederherstellung des Provider-Services warten
- **Anmeldedatenprobleme**: Provider-Anmeldedaten aktualisieren
- **Rate-Limits**: Prüfen, ob Provider-Rate-Limits überschritten werden
- **Konfigurationsprobleme**: Verifizieren, dass Provider-Konfiguration korrekt ist
- **Netzwerkprobleme**: Netzwerkverbindung zu Provider prüfen

## Leistungsprobleme

### Symptome
- Langsame Antwortzeiten
- Hohe Latenz-Metriken
- Timeout-Fehler
- Hohe Ressourcennutzung

### Diagnoseschritte

1. **Antwortzeiten prüfen**
   ```promql
   # Send-Dauer prüfen
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # Redis-Latenz prüfen
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **Ressourcennutzung prüfen**
   ```bash
   # CPU und Speicher prüfen
   top -p $(pgrep herald)
   
   # Goroutine-Anzahl prüfen
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
   ```

3. **Anforderungsmuster überprüfen**
   - Anforderungsrate prüfen
   - Spitzennutzungszeiten identifizieren
   - Auf Verkehrsspitzen prüfen

4. **Redis-Leistung prüfen**
   ```bash
   # Redis-Slow-Log prüfen
   redis-cli SLOWLOG GET 10
   
   # Redis-Speicher prüfen
   redis-cli INFO memory
   ```

### Lösungen

- **Ressourcen skalieren**: CPU/Speicher-Zuteilung erhöhen
- **Redis optimieren**: Redis Cluster verwenden oder Abfragen optimieren
- **Provider-Optimierung**: Schnellere Provider verwenden oder Provider-Aufrufe optimieren
- **Caching**: Caching für häufig zugängliche Daten implementieren
- **Lastausgleich**: Last über mehrere Instanzen verteilen

## Hilfe erhalten

Wenn Sie ein Problem nicht lösen können:

1. **Protokolle prüfen**: Herald-Protokolle auf detaillierte Fehlermeldungen überprüfen
2. **Metriken prüfen**: Prometheus-Metriken auf Muster überprüfen
3. **Dokumentation überprüfen**: [API.md](API.md) und [DEPLOYMENT.md](DEPLOYMENT.md) prüfen
4. **Problem melden**: Erstellen Sie ein Problem mit:
   - Fehlermeldungen und Protokollen
   - Konfiguration (ohne Geheimnisse)
   - Schritten zur Reproduktion
   - Erwartetem vs. tatsächlichem Verhalten

## Verwandte Dokumentation

- [API-Dokumentation](API.md) - API-Endpunkt-Details
- [Bereitstellungsleitfaden](DEPLOYMENT.md) - Bereitstellung und Konfiguration
- [Monitoring-Leitfaden](MONITORING.md) - Monitoring und Metriken
