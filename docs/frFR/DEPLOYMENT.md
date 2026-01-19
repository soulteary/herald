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

| Variable | Description | Par Défaut | Requis |
|----------|-------------|------------|--------|
| `PORT` | Port du serveur (peut être avec ou sans deux-points, par exemple `8082` ou `:8082`) | `:8082` | Non |
| `REDIS_ADDR` | Adresse Redis | `localhost:6379` | Non |
| `REDIS_PASSWORD` | Mot de passe Redis | `` | Non |
| `REDIS_DB` | Base de données Redis | `0` | Non |
| `API_KEY` | Clé API pour l'authentification | `` | Recommandé |
| `HMAC_SECRET` | Secret HMAC pour l'authentification sécurisée | `` | Optionnel |
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
| `SMTP_HOST` | Hôte du serveur SMTP | `` | Pour l'e-mail |
| `SMTP_PORT` | Port du serveur SMTP | `587` | Pour l'e-mail |
| `SMTP_USER` | Nom d'utilisateur SMTP | `` | Pour l'e-mail |
| `SMTP_PASSWORD` | Mot de passe SMTP | `` | Pour l'e-mail |
| `SMTP_FROM` | Adresse d'expéditeur SMTP | `` | Pour l'e-mail |
| `SMS_PROVIDER` | Fournisseur SMS | `` | Pour SMS |
| `ALIYUN_ACCESS_KEY` | Clé d'accès Aliyun | `` | Pour Aliyun SMS |
| `ALIYUN_SECRET_KEY` | Clé secrète Aliyun | `` | Pour Aliyun SMS |
| `ALIYUN_SIGN_NAME` | Nom de signature SMS Aliyun | `` | Pour Aliyun SMS |
| `ALIYUN_TEMPLATE_CODE` | Code de modèle SMS Aliyun | `` | Pour Aliyun SMS |

## Intégration avec Stargate

1. Définir `HERALD_URL` dans la configuration Stargate
2. Définir `HERALD_API_KEY` dans la configuration Stargate
3. Définir `HERALD_ENABLED=true` dans la configuration Stargate

Exemple :
```bash
export HERALD_URL=http://herald:8082
export HERALD_API_KEY=your-secret-key
export HERALD_ENABLED=true
```

## Sécurité

- Utiliser l'authentification HMAC pour la production
- Définir des clés API fortes
- Utiliser TLS/HTTPS en production
- Configurer les limites de débit de manière appropriée
- Surveiller Redis pour les activités suspectes
