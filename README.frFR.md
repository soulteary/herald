# Herald - Service OTP et Codes de Vérification

> **📧 Votre Passerelle vers la Vérification Sécurisée**

## 🌐 Documentation Multilingue

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald est un service OTP et codes de vérification autonome prêt pour la production qui envoie des codes de vérification par e-mail et SMS. Il dispose de limitation du débit intégrée, de contrôles de sécurité et de journalisation d'audit. Herald est conçu pour fonctionner de manière indépendante et peut être intégré avec d'autres services si nécessaire.

## Fonctionnalités Principales

- 🔒 **Sécurité par Conception** : Vérification basée sur les défis avec stockage de hachage Argon2, plusieurs méthodes d'authentification (mTLS, HMAC, API Key)
- 📊 **Limitation du Débit Intégrée** : Limitation du débit multidimensionnelle (par utilisateur, par IP, par destination) avec seuils configurables
- 📝 **Piste d'Audit Complète** : Journalisation d'audit complète pour toutes les opérations avec suivi des fournisseurs
- 🔌 **Fournisseurs Extensibles** : Architecture de fournisseurs d'e-mail et SMS extensible

## Démarrage Rapide

### Utilisation de Docker Compose

Le moyen le plus simple de commencer est avec Docker Compose, qui inclut Redis :

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

Réponse attendue :
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Tester l'API

Créer un défi de test (nécessite une authentification - voir [Documentation API](docs/frFR/API.md)) :

```bash
# Set your API key (from docker-compose.yml: your-secret-api-key-here)
export API_KEY="your-secret-api-key-here"

# Create a challenge
curl -X POST http://localhost:8082/v1/otp/challenges \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "channel": "email",
    "destination": "user@example.com",
    "purpose": "login"
  }'
```

### Afficher les Logs

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### Déploiement Manuel

Pour le déploiement manuel et la configuration avancée, voir le [Guide de Déploiement](docs/frFR/DEPLOYMENT.md).

## Configuration de Base

Herald nécessite une configuration minimale pour commencer :

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

Pour les options de configuration complètes, y compris les limites de débit, l'expiration des défis et les paramètres des fournisseurs, voir le [Guide de Déploiement](docs/frFR/DEPLOYMENT.md#configuration).

## Documentation

### Pour les Développeurs

- **[Documentation API](docs/frFR/API.md)** - Référence API complète avec méthodes d'authentification, points de terminaison et codes d'erreur
- **[Guide de Déploiement](docs/frFR/DEPLOYMENT.md)** - Options de configuration, déploiement Docker et exemples d'intégration

### Pour les Opérations

- **[Guide de Surveillance](docs/frFR/MONITORING.md)** - Métriques Prometheus, tableaux de bord Grafana et alertes
- **[Guide de Dépannage](docs/frFR/TROUBLESHOOTING.md)** - Problèmes courants, étapes de diagnostic et solutions

### Index de Documentation

Pour un aperçu complet de toute la documentation, voir [docs/frFR/README.md](docs/frFR/README.md).

## License

See [LICENSE](LICENSE) for details.
