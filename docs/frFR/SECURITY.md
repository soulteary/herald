# Documentation de Sécurité

> 🌐 **Language / 语言**: [English](../enUS/SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](../koKR/SECURITY.md)

Ce document explique les fonctionnalités de sécurité de Herald, la configuration de sécurité et les meilleures pratiques.

> ⚠️ **Note**: Cette documentation est en cours de traduction. Pour la version complète, consultez la [version anglaise](../enUS/SECURITY.md).

## Fonctionnalités de Sécurité Implémentées

1. **Vérification basée sur Challenge**: Utilise le modèle challenge-verify pour prévenir les attaques de rejeu et garantir l'utilisation unique des codes de vérification
2. **Stockage sécurisé des codes**: Les codes de vérification sont stockés sous forme de hachages Argon2, jamais en texte clair
3. **Limitation du débit multidimensionnelle**: Limitation du débit par user_id, destination (email/téléphone) et adresse IP pour prévenir les abus
4. **Authentification de service**: Prend en charge mTLS, signature HMAC et authentification par clé API pour la communication inter-services
5. **Protection d'idempotence**: Empêche la création de challenges en double et l'envoi de codes en double à l'aide de clés d'idempotence
6. **Expiration des challenges**: Expiration automatique des challenges avec TTL configurable
7. **Limitation des tentatives**: Limites maximales de tentatives par challenge pour prévenir les attaques par force brute
8. **Refroidissement de renvoi**: Empêche le renvoi rapide des codes de vérification
9. **Journalisation d'audit**: Piste d'audit complète pour toutes les opérations, y compris les envois, vérifications et échecs
10. **Sécurité du fournisseur**: Communication sécurisée avec les fournisseurs d'email et SMS

Pour plus de détails, consultez la [version anglaise](../enUS/SECURITY.md).

## Signalement de Vulnérabilité

Si vous découvrez une vulnérabilité de sécurité, veuillez la signaler via:

1. **GitHub Security Advisory** (Préféré)
   - Allez dans l'onglet [Security](https://github.com/soulteary/herald/security) du dépôt
   - Cliquez sur "Report a vulnerability"
   - Remplissez le formulaire de conseil de sécurité

2. **Email** (Si GitHub Security Advisory n'est pas disponible)
   - Envoyez un email aux mainteneurs du projet
   - Incluez une description détaillée de la vulnérabilité

**Veuillez ne pas signaler les vulnérabilités de sécurité via les problèmes GitHub publics.**
