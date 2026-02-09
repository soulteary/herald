# Guide de Déploiement Herald

## Démarrage Rapide

### Utilisation de Docker Compose

```bash
cd herald
docker-compose up -d
```

### Déploiement Manuel

```bash
# Construire
go build -o herald main.go

# Exécuter
./herald
```

## Configuration

### Variables d'Environnement

**Liste complète (alignée avec le code) :** [Anglais](enUS/DEPLOYMENT.md#environment-variables) | [中文](zhCN/DEPLOYMENT.md#环境变量)

| Variable | Description | Par Défaut | Requis |
|----------|-------------|------------|--------|
| `PORT` | Port du serveur (peut être avec ou sans deux-points, par exemple `8082` ou `:8082`) | `:8082` | Non |
| `REDIS_ADDR` | Adresse Redis | `localhost:6379` | Non |
| `REDIS_PASSWORD` | Mot de passe Redis | `` | Non |
| `REDIS_DB` | Base de données Redis | `0` | Non |
| `API_KEY` | Clé API pour l'authentification | `` | Recommandé |
| `HMAC_SECRET` | Secret HMAC pour l'authentification sécurisée | `` | Optionnel |
| `HERALD_HMAC_KEYS` | Clés HMAC multiples (JSON) | `` | Optionnel |
| `LOG_LEVEL` | Niveau de journalisation | `info` | Non |
| `CHALLENGE_EXPIRY` | Expiration du défi | `5m` | Non |
| `MAX_ATTEMPTS` | Nombre maximum de tentatives de vérification | `5` | Non |
| `RESEND_COOLDOWN` | Délai de réenvoi | `60s` | Non |
| `CODE_LENGTH` | Longueur du code de vérification | `6` | Non |
| `RATE_LIMIT_PER_USER` | Limite de débit par utilisateur/heure | `10` | Non |
| `RATE_LIMIT_PER_IP` | Limite de débit par IP/minute | `5` | Non |
| `RATE_LIMIT_PER_DESTINATION` | Limite de débit par destination/heure | `10` | Non |
| `LOCKOUT_DURATION` | Durée de verrouillage de l'utilisateur après le nombre maximum de tentatives | `10m` | Non |
| `SERVICE_NAME` | Identifiant de service pour l'authentification HMAC | `herald` | Non |
| `SMTP_HOST` | Hôte du serveur SMTP | `` | Pour l'e-mail (intégré) |
| `SMTP_PORT` | Port du serveur SMTP | `587` | Pour l'e-mail |
| `SMTP_USER` | Nom d'utilisateur SMTP | `` | Pour l'e-mail |
| `SMTP_PASSWORD` | Mot de passe SMTP | `` | Pour l'e-mail |
| `SMTP_FROM` | Adresse d'expéditeur SMTP | `` | Pour l'e-mail |
| `SMS_PROVIDER` | Fournisseur SMS (ex. nom pour les logs) | `` | Pour SMS |
| `SMS_API_BASE_URL` | URL de base de l'API HTTP SMS | `` | Pour SMS (API HTTP) |
| `SMS_API_KEY` | Clé API SMS | `` | Pour SMS (optionnel) |
| `HERALD_DINGTALK_API_URL` | URL de base de [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (ex. `http://herald-dingtalk:8083`) | `` | Pour le canal DingTalk |
| `HERALD_DINGTALK_API_KEY` | Clé API optionnelle ; doit correspondre à `API_KEY` de herald-dingtalk si défini | `` | Non |
| `HERALD_SMTP_API_URL` | URL de base de [herald-smtp](https://github.com/soulteary/herald-smtp) (ex. `http://herald-smtp:8084`) ; si défini, le SMTP intégré n'est pas utilisé | `` | Pour le canal e-mail (optionnel) |
| `HERALD_SMTP_API_KEY` | Clé API optionnelle ; doit correspondre à `API_KEY` de herald-smtp si défini | `` | Non |
| `HERALD_TEST_MODE` | Si `true` : code debug dans Redis/réponse. **Uniquement pour les tests ; en production toujours `false`.** | `false` | Non |

### Canal e-mail (herald-smtp)

Lorsque `HERALD_SMTP_API_URL` est défini, Herald n'utilise pas le SMTP intégré. Il transmet l'envoi d'e-mails à [herald-smtp](https://github.com/soulteary/herald-smtp) via HTTP. Toutes les identifiants et la logique SMTP sont dans herald-smtp ; Herald ne stocke aucune identifiant SMTP pour le canal e-mail dans ce mode. Définissez `HERALD_SMTP_API_URL` sur l'URL de base de votre service herald-smtp. Si herald-smtp est configuré avec `API_KEY`, définissez `HERALD_SMTP_API_KEY` sur la même valeur. Lorsque `HERALD_SMTP_API_URL` est défini, Herald ignore `SMTP_HOST` et les paramètres SMTP intégrés associés.

### Canal DingTalk (herald-dingtalk)

Lorsque `channel` est `dingtalk`, Herald n'envoie pas les messages lui-même ; il transmet l'envoi à [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) via HTTP. Toutes les identifiants et la logique DingTalk sont dans herald-dingtalk ; Herald ne stocke aucun identifiant DingTalk. Définissez `HERALD_DINGTALK_API_URL` sur l'URL de base de votre service herald-dingtalk. Si herald-dingtalk est configuré avec `API_KEY`, définissez `HERALD_DINGTALK_API_KEY` sur la même valeur.

## Intégration avec d'autres services (Optionnel)

Herald est conçu pour fonctionner de manière indépendante et peut être intégré avec d'autres services si nécessaire. Si vous souhaitez intégrer Herald avec d'autres services d'authentification ou de passerelle, vous pouvez configurer ce qui suit :

**Exemple de configuration d'intégration :**
```bash
# URL du service où Herald est accessible
export HERALD_URL=http://herald:8082

# Clé API pour l'authentification entre services
export HERALD_API_KEY=your-secret-key

# Activer l'intégration Herald (si votre service le supporte)
export HERALD_ENABLED=true
```

**Note** : Herald peut être utilisé de manière autonome sans dépendances de services externes. L'intégration avec d'autres services est optionnelle et dépend de votre cas d'utilisation spécifique.

## Sécurité

- Utiliser l'authentification HMAC pour la production
- Définir des clés API fortes
- Utiliser TLS/HTTPS en production
- Configurer les limites de débit de manière appropriée
- Surveiller Redis pour les activités suspectes
