# Herald - Service OTP et Codes de Vérification

> **📧 Votre Passerelle vers la Vérification Sécurisée**

## 🌐 Documentation Multilingue

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald est un service léger prêt pour la production permettant d'envoyer des codes de vérification (OTP) par e-mail (la prise en charge SMS est actuellement en développement), avec limitation du débit intégrée, contrôles de sécurité et journalisation d'audit.

## Fonctionnalités

- 🚀 **Haute Performance** : Construit avec Go et Fiber
- 🔒 **Sécurisé** : Vérification basée sur les défis avec stockage de hachage
- 📊 **Limitation du Débit** : Limitation du débit multidimensionnelle (par utilisateur, par IP, par destination)
- 📝 **Journalisation d'Audit** : Piste d'audit complète pour toutes les opérations
- 🔌 **Fournisseurs Extensibles** : Prise en charge des fournisseurs d'e-mail (les fournisseurs SMS sont des implémentations de remplacement et ne sont pas encore entièrement fonctionnels)
- ⚡ **Backend Redis** : Stockage rapide et distribué avec Redis

## Démarrage Rapide

```bash
# Exécuter avec Docker Compose
docker-compose up -d

# Ou exécuter directement
go run main.go
```

## Configuration

Définir les variables d'environnement :

- `PORT` : Port du serveur (par défaut : `:8082`)
- `REDIS_ADDR` : Adresse Redis (par défaut : `localhost:6379`)
- `REDIS_PASSWORD` : Mot de passe Redis (optionnel)
- `REDIS_DB` : Numéro de base de données Redis (par défaut : `0`)
- `API_KEY` : Clé API pour l'authentification inter-services
- `LOG_LEVEL` : Niveau de journalisation (par défaut : `info`)

Pour les options de configuration complètes, voir [DEPLOYMENT.md](docs/frFR/DEPLOYMENT.md).

## Documentation API

Voir [API.md](docs/frFR/API.md) pour la documentation API détaillée.
