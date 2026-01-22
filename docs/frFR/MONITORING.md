# Guide de Monitoring Herald

Herald expose des métriques Prometheus pour le monitoring et l'observabilité. Ce document décrit toutes les métriques disponibles, comment configurer le monitoring et les meilleures pratiques.

## Point de Terminaison des Métriques

Herald expose les métriques Prometheus au point de terminaison `/metrics` :

```
GET http://localhost:8082/metrics
```

## Métriques Disponibles

### Métriques de Challenge

#### `herald_otp_challenges_total`

Compteur suivant le nombre total de challenges OTP créés.

**Labels:**
- `channel`: Type de canal (`sms` ou `email`)
- `purpose`: Objectif du challenge (par ex. `login`, `reset`, `bind`)
- `result`: Résultat de l'opération (`success` ou `failed`)

**Exemple:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### Métriques d'Envoi

#### `herald_otp_sends_total`

Compteur suivant le nombre total d'envois OTP via les fournisseurs.

**Labels:**
- `channel`: Type de canal (`sms` ou `email`)
- `provider`: Nom du fournisseur (par ex. `smtp`, `aliyun`, `placeholder`)
- `result`: Résultat de l'opération d'envoi (`success` ou `failed`)

**Exemple:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

Histogramme suivant la durée des opérations d'envoi OTP.

**Labels:**
- `provider`: Nom du fournisseur

**Buckets:** Buckets Prometheus par défaut (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Exemple:**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### Métriques de Vérification

#### `herald_otp_verifications_total`

Compteur suivant le nombre total de vérifications OTP.

**Labels:**
- `result`: Résultat de la vérification (`success` ou `failed`)
- `reason`: Raison de l'échec (par ex. `expired`, `invalid`, `locked`, `too_many_attempts`)

**Exemple:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### Métriques de Limitation de Débit

#### `herald_rate_limit_hits_total`

Compteur suivant le nombre total de hits de limitation de débit.

**Labels:**
- `scope`: Portée de la limitation de débit (`user`, `ip`, `destination`, `resend_cooldown`)

**Exemple:**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Métriques Redis

#### `herald_redis_latency_seconds`

Histogramme suivant la latence des opérations Redis.

**Labels:**
- `operation`: Type d'opération Redis (`get`, `set`, `del`, `exists`)

**Buckets:** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**Exemple:**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Configuration Prometheus

### Configuration de Scrape de Base

Ajoutez ce qui suit à votre `prometheus.yml` :

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
```

### Découverte de Service

Pour les déploiements Kubernetes, utilisez la découverte de service :

```yaml
scrape_configs:
  - job_name: 'herald'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - default
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: herald
        action: keep
      - source_labels: [__meta_kubernetes_pod_ip]
        target_label: __address__
        replacement: '${1}:8082'
```

## Métriques Clés à Surveiller

### Métriques Métier

1. **Taux de Création de Challenge**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **Taux de Réussite de Challenge**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **Taux de Réussite de Vérification**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **Taux de Réussite d'Envoi par Fournisseur**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### Métriques de Performance

1. **Durée Moyenne d'Envoi**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **Durée d'Envoi P95**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Latence Redis P99**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### Métriques d'Erreur

1. **Taux de Hit de Limitation de Débit**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **Raisons d'Échec de Vérification**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **Taux d'Échec d'Envoi**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## Règles d'Alerte

### Exemples de Règles d'Alerte

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # Taux d'échec élevé
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Le taux d'échec de vérification Herald est élevé"
          description: "Le taux d'échec de vérification est {{ $value | humanizePercentage }}"

      # Taux d'échec d'envoi élevé
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Le taux d'échec d'envoi Herald est élevé"
          description: "Le taux d'échec d'envoi est {{ $value | humanizePercentage }}"

      # Latence Redis élevée
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "La latence Redis Herald est élevée"
          description: "La latence Redis P99 est {{ $value }}s"

      # Hits de limitation de débit élevés
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Les hits de limitation de débit Herald sont élevés"
          description: "Le taux de hit de limitation de débit est {{ $value }} hits/seconde"
```

## Tableaux de Bord Grafana

### Panneaux de Tableau de Bord Recommandés

1. **Panneau de Vue d'Ensemble**
   - Taux de création de challenge
   - Taux de réussite de vérification
   - Taux de réussite d'envoi
   - Challenges actifs actuels (si suivi)

2. **Panneau de Performance**
   - Durée d'envoi (P50, P95, P99)
   - Latence Redis (P50, P95, P99)
   - Taux de requête

3. **Panneau d'Erreur**
   - Répartition des raisons d'échec de vérification
   - Taux d'échec d'envoi par fournisseur
   - Hits de limitation de débit par portée

4. **Panneau Fournisseur**
   - Taux de réussite d'envoi par fournisseur
   - Durée d'envoi par fournisseur
   - Volume d'envoi par fournisseur

## Support OpenTelemetry

**Note**: Le support OpenTelemetry pour le traçage distribué est prévu mais pas encore implémenté. Cela inclut :
- Propagation des en-têtes `traceparent` / `tracestate`
- Spans principaux : `otp.challenge.create`, `otp.provider.send`, `otp.verify`
- Tags de span : `channel`, `purpose`, `provider`, `result`, `reason`

Voir la feuille de route du projet pour le calendrier d'implémentation.

## Meilleures Pratiques

1. **Surveiller les Métriques Métier Clés**: Suivre la création de challenge, les taux de réussite de vérification et les taux de réussite d'envoi
2. **Configurer les Alertes**: Configurer les alertes pour les taux d'échec élevés et la dégradation des performances
3. **Suivre la Performance des Fournisseurs**: Surveiller la durée d'envoi et les taux de réussite par fournisseur
4. **Surveiller la Santé Redis**: Suivre la latence Redis et les problèmes de connexion
5. **Monitoring de Limitation de Débit**: Surveiller les hits de limitation de débit pour comprendre les modèles d'utilisation et les abus potentiels

## Documentation Connexe

- [Documentation API](API.md) - Détails des points de terminaison API
- [Guide de Déploiement](DEPLOYMENT.md) - Déploiement et configuration
- [Guide de Dépannage](TROUBLESHOOTING.md) - Problèmes courants et solutions
