# Index de Documentation

> **Traduction ancienne / Historical translation.** This page predates current security changes.
> Do not use its commands or configuration as current instructions. See the
> [current English guide](../enUS/README.md) or [中文](../zhCN/README.md)
> and [translation status](STALE.md). HMAC creation requires v3; builds and validation use Linux.

Bienvenue dans la documentation du service Herald OTP et de codes de vérification.

## 🌐 Documentation Multilingue

- [English](../enUS/README.md) | [中文](../zhCN/README.md) | [Français](README.md) | [Italiano](../itIT/README.md) | [日本語](../jaJP/README.md) | [Deutsch](../deDE/README.md) | [한국어](../koKR/README.md)

## 📚 Liste des Documents

### Documents Principaux

- **[README.md](../../README.frFR.md)** - Vue d'ensemble du projet et guide de démarrage rapide

### Documents Détaillés

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Architecture full-stack
  - Stargate / Warden / Herald / Redis / Fournisseur
  - Flux de données et interfaces clés (challenge, verify, envoi fournisseur)
  - Périmètres de sécurité (HMAC/mTLS, PII)

- **[API.md](API.md)** - Documentation complète des points de terminaison API
  - Méthodes d'authentification
  - Points de terminaison de vérification de santé
  - Création et vérification de défis
  - Limitation du débit
  - Codes d'erreur et réponses

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Guide de déploiement
  - Déploiement Docker Compose
  - Déploiement manuel
  - Options de configuration
  - Intégration optionnelle avec d'autres services
  - Meilleures pratiques de sécurité

- **[MONITORING.md](MONITORING.md)** - Guide de monitoring
  - Métriques Prometheus
  - Tableaux de bord Grafana
  - Règles d'alerte
  - Meilleures pratiques

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Guide de dépannage
  - Problèmes courants et solutions
  - Étapes de diagnostic
  - Optimisation des performances

## 🚀 Navigation Rapide

### Pour Commencer

1. Lisez [README.frFR.md](../../README.frFR.md) pour comprendre le projet
2. Consultez la section [Démarrage Rapide](../../README.frFR.md#démarrage-rapide)
3. Référez-vous à [Configuration](../../README.frFR.md#configuration-de-base) pour configurer le service

### Développeurs

1. Lisez [ARCHITECTURE.md](ARCHITECTURE.md) pour la vue d'ensemble full-stack
2. Consultez [API.md](API.md) pour comprendre les interfaces API
3. Examinez [DEPLOYMENT.md](DEPLOYMENT.md) pour les options de déploiement

### Opérations

1. Lisez [DEPLOYMENT.md](DEPLOYMENT.md) pour comprendre les méthodes de déploiement
2. Consultez [API.md](API.md) pour les détails des points de terminaison API
3. Référez-vous à [Sécurité](DEPLOYMENT.md#sécurité) pour les meilleures pratiques de sécurité
4. Surveiller la santé du service : [MONITORING.md](MONITORING.md)
5. Dépanner les problèmes : [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 Structure des Documents

```
herald/
├── README.md              # Document principal du projet (Anglais)
├── README.frFR.md         # Document principal du projet (Français)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # Index de documentation (Anglais)
│   │   ├── ARCHITECTURE.md # Architecture full-stack (Anglais)
│   │   ├── API.md          # Document API (Anglais)
│   │   ├── DEPLOYMENT.md   # Guide de déploiement (Anglais)
│   │   ├── MONITORING.md   # Guide de monitoring (Anglais)
│   │   └── TROUBLESHOOTING.md # Guide de dépannage (Anglais)
│   └── frFR/
│       ├── README.md       # Index de documentation (Français, ce fichier)
│       ├── ARCHITECTURE.md # Architecture full-stack (Français)
│       ├── API.md          # Document API (Français)
│       ├── DEPLOYMENT.md   # Guide de déploiement (Français)
│       ├── MONITORING.md   # Guide de monitoring (Français)
│       └── TROUBLESHOOTING.md # Guide de dépannage (Français)
└── ...
```

## 🔍 Recherche par Sujet

### Lié à l'API

- Liste des points de terminaison API : [API.md](API.md)
- Méthodes d'authentification : [API.md#authentification](API.md#authentification)
- Gestion des erreurs : [API.md#codes-derreur](API.md#codes-derreur)
- Limitation du débit : [API.md#limitation-du-débit](API.md#limitation-du-débit)

### Lié au Déploiement

- Déploiement Docker : [DEPLOYMENT.md#démarrage-rapide](DEPLOYMENT.md#démarrage-rapide)
- Options de configuration : [DEPLOYMENT.md#configuration](DEPLOYMENT.md#configuration)
- Intégration de services : [DEPLOYMENT.md#intégration-avec-dautres-services-optionnel](DEPLOYMENT.md#intégration-avec-dautres-services-optionnel)
- Sécurité : [DEPLOYMENT.md#sécurité](DEPLOYMENT.md#sécurité)

### Monitoring et Opérations

- Métriques Prometheus : [MONITORING.md](MONITORING.md)
- Tableaux de bord Grafana : [MONITORING.md#tableaux-de-bord-grafana](MONITORING.md#tableaux-de-bord-grafana)
- Dépannage : [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 Recommandations d'Utilisation

1. **Utilisateurs pour la première fois** : Commencez par [README.frFR.md](../../README.frFR.md) et suivez le guide de démarrage rapide
2. **Configurer le service** : Référez-vous à [DEPLOYMENT.md](DEPLOYMENT.md) pour comprendre toutes les options de configuration
3. **Intégrer avec les services** : Consultez la section d'intégration dans [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Intégration API** : Lisez [API.md](API.md) pour comprendre les interfaces API
5. **Surveiller le service** : Configurez le monitoring avec [MONITORING.md](MONITORING.md)
6. **Dépanner les problèmes** : Référez-vous à [TROUBLESHOOTING.md](TROUBLESHOOTING.md) pour les problèmes courants

## 📝 Mises à Jour des Documents

La documentation est continuellement mise à jour au fur et à mesure de l'évolution du projet. Si vous trouvez des erreurs ou avez besoin d'ajouts, veuillez soumettre un Issue ou une Pull Request.

## 🤝 Contribution

Les améliorations de la documentation sont les bienvenues :

1. Trouvez des erreurs ou des domaines à améliorer
2. Soumettez un Issue décrivant le problème
3. Ou soumettez directement une Pull Request
