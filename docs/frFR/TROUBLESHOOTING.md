# Guide de Dépannage Herald

Ce guide vous aide à diagnostiquer et résoudre les problèmes courants avec le service Herald OTP et codes de vérification.

## Table des Matières

- [Code de Vérification Non Reçu](#code-de-vérification-non-reçu)
- [Erreurs de Code de Vérification](#erreurs-de-code-de-vérification)
- [Erreurs 401 Non Autorisées](#erreurs-401-non-autorisées)
- [Problèmes de Limitation de Débit](#problèmes-de-limitation-de-débit)
- [Problèmes de Connexion Redis](#problèmes-de-connexion-redis)
- [Échecs d'Envoi du Fournisseur](#échecs-denvoi-du-fournisseur)
- [Problèmes de Performance](#problèmes-de-performance)

## Code de Vérification Non Reçu

### Symptômes
- L'utilisateur signale ne pas avoir reçu le code de vérification par SMS ou e-mail
- La création du challenge réussit mais aucun code n'est livré

### Étapes de Diagnostic

1. **Vérifier la Connexion du Fournisseur**
   ```bash
   # Vérifier les logs Herald pour les erreurs du fournisseur
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **Vérifier la Configuration du Fournisseur**
   - Vérifier les paramètres SMTP (SMTP intégré) : `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - Pour e-mail via herald-smtp : s'assurer que `HERALD_SMTP_API_URL` est défini et que herald-smtp est accessible ; optionnellement `HERALD_SMTP_API_KEY`. Si les e-mails ne sont pas reçus, vérifier les logs et la configuration SMTP de herald-smtp.
   - Vérifier les paramètres du fournisseur SMS : `SMS_PROVIDER`, `ALIYUN_ACCESS_KEY`, etc.
   - Vérifier que les identifiants du fournisseur sont corrects

3. **Vérifier les Métriques Prometheus**
   ```promql
   # Vérifier le taux d'échec d'envoi
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # Vérifier le succès d'envoi par fournisseur
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **Vérifier les Logs d'Audit**
   - Examiner les logs d'audit pour les événements `send_failed`
   - Vérifier les codes d'erreur et messages du fournisseur
   - Vérifier que les adresses de destination sont valides

5. **Tester le Fournisseur Directement**
   - Tester la connexion SMTP manuellement
   - Tester l'API du fournisseur SMS directement
   - Vérifier la connectivité réseau aux points de terminaison du fournisseur

### Solutions

- **Problèmes de Configuration du Fournisseur**: Mettre à jour les identifiants et paramètres du fournisseur
- **Problèmes Réseau**: Vérifier les règles de pare-feu et la connectivité réseau
- **Limites de Débit du Fournisseur**: Vérifier si le fournisseur a des limites de débit qui sont dépassées
- **Destinations Invalides**: Vérifier que les adresses e-mail et numéros de téléphone sont valides

## Erreurs de Code de Vérification

### Symptômes
- L'utilisateur signale des erreurs "code invalide"
- La vérification échoue même avec le code correct
- Le challenge apparaît expiré ou verrouillé

### Étapes de Diagnostic

1. **Vérifier le Statut du Challenge**
   ```bash
   # Vérifier Redis pour les données du challenge
   redis-cli GET "otp:ch:{challenge_id}"
   ```

2. **Vérifier l'Expiration du Challenge**
   - Vérifier la configuration `CHALLENGE_EXPIRY` (par défaut : 5 minutes)
   - Vérifier que l'heure système est synchronisée (NTP)
   - Vérifier si le challenge a expiré

3. **Vérifier le Nombre de Tentatives**
   - Vérifier la configuration `MAX_ATTEMPTS` (par défaut : 5)
   - Vérifier si le challenge est verrouillé en raison de trop de tentatives
   - Examiner les logs d'audit pour l'historique des tentatives

4. **Vérifier les Métriques Prometheus**
   ```promql
   # Vérifier les raisons d'échec de vérification
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # Vérifier les challenges verrouillés
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
   ```

5. **Examiner les Logs d'Audit**
   - Vérifier les tentatives et résultats de vérification
   - Vérifier que le format du code correspond au format attendu
   - Vérifier les problèmes de timing

### Solutions

- **Challenge Expiré**: L'utilisateur doit demander un nouveau code de vérification
- **Trop de Tentatives**: Le challenge est verrouillé, attendre la durée de verrouillage ou demander un nouveau challenge
- **Format de Code Incompatible**: Vérifier que la longueur du code correspond à la configuration `CODE_LENGTH`
- **Synchronisation Temporelle**: S'assurer que les horloges système sont synchronisées

## Erreurs 401 Non Autorisées

### Symptômes
- Les requêtes API retournent 401 Non Autorisé
- Échecs d'authentification dans les logs
- La communication service-à-service échoue

### Étapes de Diagnostic

1. **Vérifier la Méthode d'Authentification**
   - Vérifier quelle méthode d'authentification est utilisée (mTLS, HMAC, Clé API)
   - Vérifier si les en-têtes d'authentification sont présents

2. **Pour l'Authentification HMAC**
   ```bash
   # Vérifier que HMAC_SECRET est configuré
   echo $HMAC_SECRET
   
   # Vérifier que l'horodatage est dans la fenêtre de 5 minutes
   # Vérifier que le calcul de signature correspond à l'implémentation de Herald
   ```

3. **Pour l'Authentification par Clé API**
   ```bash
   # Vérifier que API_KEY est défini
   echo $API_KEY
   
   # Vérifier que l'en-tête X-API-Key correspond à la clé configurée
   ```

4. **Pour l'Authentification mTLS**
   - Vérifier que le certificat client est valide
   - Vérifier que la chaîne de certificats est de confiance
   - Vérifier que la connexion TLS est établie

5. **Vérifier les Logs Herald**
   ```bash
   # Vérifier les erreurs d'authentification
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### Solutions

- **Identifiants Manquants**: Définir les variables d'environnement `API_KEY` ou `HMAC_SECRET`
- **Signature Invalide**: Vérifier que le calcul de signature HMAC correspond à l'implémentation de Herald
- **Horodatage Expiré**: S'assurer que les horloges client et serveur sont synchronisées (dans les 5 minutes)
- **Problèmes de Certificat**: Vérifier que les certificats mTLS sont valides et de confiance

## Problèmes de Limitation de Débit

### Symptômes
- Les utilisateurs signalent des erreurs "limite de débit dépassée"
- Les requêtes légitimes sont bloquées
- Métriques élevées de hits de limitation de débit

### Étapes de Diagnostic

1. **Vérifier la Configuration de Limitation de Débit**
   ```bash
   # Vérifier les paramètres de limitation de débit
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **Vérifier les Métriques Prometheus**
   ```promql
   # Vérifier les hits de limitation de débit par portée
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # Vérifier le taux de hits de limitation de débit
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Examiner les Clés Redis**
   ```bash
   # Vérifier les clés de limitation de débit
   redis-cli KEYS "otp:rate:*"
   
   # Vérifier une clé de limitation de débit spécifique
   redis-cli GET "otp:rate:user:{user_id}"
   ```

4. **Analyser les Modèles d'Utilisation**
   - Vérifier si les utilisateurs légitimes atteignent les limites
   - Identifier les abus potentiels ou l'activité de bot
   - Examiner les modèles de création de challenge

### Solutions

- **Ajuster les Limites de Débit**: Augmenter les limites si les utilisateurs légitimes sont affectés
- **Éducation des Utilisateurs**: Informer les utilisateurs sur les limites de débit et les périodes de refroidissement
- **Prévention des Abus**: Mettre en œuvre des mesures de sécurité supplémentaires pour les abus suspectés
- **Refroidissement de Renvoi**: Les utilisateurs doivent attendre la période `RESEND_COOLDOWN` (par défaut : 60 secondes)

## Problèmes de Connexion Redis

### Symptômes
- Le service ne démarre pas
- Le contrôle de santé retourne malsain
- La création/vérification de challenge échoue
- Métriques élevées de latence Redis

### Étapes de Diagnostic

1. **Vérifier la Connectivité Redis**
   ```bash
   # Tester la connexion Redis
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **Vérifier la Configuration**
   ```bash
   # Vérifier la configuration Redis
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   echo $REDIS_DB
   ```

3. **Vérifier la Santé Redis**
   ```bash
   # Vérifier les informations Redis
   redis-cli INFO
   
   # Vérifier l'utilisation de la mémoire
   redis-cli INFO memory
   ```

4. **Vérifier les Métriques Prometheus**
   ```promql
   # Vérifier la latence Redis
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # Vérifier les erreurs d'opération Redis
   rate(herald_redis_latency_seconds_count[5m])
   ```

5. **Examiner les Logs Herald**
   ```bash
   # Vérifier les erreurs Redis
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### Solutions

- **Problèmes de Connexion**: Vérifier que `REDIS_ADDR` est correct et que Redis est accessible
- **Problèmes d'Authentification**: Vérifier que `REDIS_PASSWORD` est correct
- **Sélection de Base de Données**: Vérifier que `REDIS_DB` est correct (par défaut : 0)
- **Problèmes de Performance**: Vérifier l'utilisation de la mémoire Redis et envisager la mise à l'échelle
- **Problèmes Réseau**: Vérifier la connectivité réseau et les règles de pare-feu

## Échecs d'Envoi du Fournisseur

### Symptômes
- Taux d'échec d'envoi élevé dans les métriques
- Erreurs `send_failed` dans les logs
- Messages d'erreur spécifiques au fournisseur

### Étapes de Diagnostic

1. **Vérifier les Métriques du Fournisseur**
   ```promql
   # Vérifier le taux d'échec d'envoi par fournisseur
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # Vérifier la durée d'envoi par fournisseur
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **Examiner les Logs du Fournisseur**
   - Vérifier les messages d'erreur spécifiques au fournisseur
   - Examiner les logs d'audit pour les erreurs du fournisseur
   - Vérifier les codes de réponse de l'API du fournisseur

3. **Tester la Connexion du Fournisseur**
   - Tester la connexion SMTP manuellement
   - Tester l'API du fournisseur SMS directement
   - Vérifier les identifiants du fournisseur

4. **Vérifier le Statut du Fournisseur**
   - Vérifier la page de statut du fournisseur
   - Vérifier que l'API du fournisseur est opérationnelle
   - Vérifier les limites de débit du fournisseur

### Solutions

- **Panne du Fournisseur**: Attendre que le fournisseur restaure le service
- **Problèmes d'Identifiants**: Mettre à jour les identifiants du fournisseur
- **Limites de Débit**: Vérifier si les limites de débit du fournisseur sont dépassées
- **Problèmes de Configuration**: Vérifier que la configuration du fournisseur est correcte
- **Problèmes Réseau**: Vérifier la connectivité réseau au fournisseur

## Problèmes de Performance

### Symptômes
- Temps de réponse lents
- Métriques de latence élevées
- Erreurs de timeout
- Utilisation élevée des ressources

### Étapes de Diagnostic

1. **Vérifier les Temps de Réponse**
   ```promql
   # Vérifier la durée d'envoi
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # Vérifier la latence Redis
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **Vérifier l'Utilisation des Ressources**
   ```bash
   # Vérifier le CPU et la mémoire
   top -p $(pgrep herald)
   
   # Vérifier le nombre de Goroutines
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
   ```

3. **Examiner les Modèles de Requête**
   - Vérifier le taux de requête
   - Identifier les heures de pointe d'utilisation
   - Vérifier les pics de trafic

4. **Vérifier les Performances Redis**
   ```bash
   # Vérifier le journal lent de Redis
   redis-cli SLOWLOG GET 10
   
   # Vérifier la mémoire Redis
   redis-cli INFO memory
   ```

### Solutions

- **Mettre à l'Échelle les Ressources**: Augmenter l'allocation CPU/mémoire
- **Optimiser Redis**: Utiliser Redis Cluster ou optimiser les requêtes
- **Optimisation du Fournisseur**: Utiliser des fournisseurs plus rapides ou optimiser les appels au fournisseur
- **Mise en Cache**: Implémenter la mise en cache pour les données fréquemment accédées
- **Équilibrage de Charge**: Distribuer la charge sur plusieurs instances

## Obtenir de l'Aide

Si vous ne parvenez pas à résoudre un problème :

1. **Vérifier les Logs**: Examiner les logs Herald pour les messages d'erreur détaillés
2. **Vérifier les Métriques**: Examiner les métriques Prometheus pour les modèles
3. **Examiner la Documentation**: Vérifier [API.md](API.md) et [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Ouvrir un Problème**: Créer un problème avec :
   - Messages d'erreur et logs
   - Configuration (sans secrets)
   - Étapes pour reproduire
   - Comportement attendu vs réel

## Documentation Connexe

- [Documentation API](API.md) - Détails des points de terminaison API
- [Guide de Déploiement](DEPLOYMENT.md) - Déploiement et configuration
- [Guide de Monitoring](MONITORING.md) - Monitoring et métriques
