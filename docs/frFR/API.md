# Documentation API Herald

Herald est un service de codes de vérification et OTP qui gère l'envoi de codes de vérification via SMS et e-mail, avec limitation du débit intégrée et contrôles de sécurité.

## URL de Base

```
http://localhost:8082
```

## Authentification

Herald prend en charge trois méthodes d'authentification dans l'ordre de priorité suivant :

1. **mTLS** (Le plus sécurisé) : TLS mutuel avec vérification du certificat client (priorité la plus élevée)
2. **Signature HMAC** (Sécurisé) : Définir les en-têtes `X-Signature`, `X-Timestamp` et `X-Service`
3. **Clé API** (Simple) : Définir l'en-tête `X-API-Key` (priorité la plus faible)

### Authentification mTLS

Lors de l'utilisation de HTTPS avec un certificat client vérifié, Herald authentifiera automatiquement la requête via mTLS. C'est la méthode la plus sécurisée et elle a la priorité sur les autres méthodes d'authentification.

### Signature HMAC

La signature HMAC est calculée comme suit :
```
HMAC-SHA256(timestamp:service:body, secret)
```

Où :
- `timestamp` : Horodatage Unix (secondes)
- `service` : Identifiant de service (par exemple, "my-service", "api-gateway")
- `body` : Corps de la requête (chaîne JSON)
- `secret` : Clé secrète HMAC

**Note** : L'horodatage doit être dans les 5 minutes (300 secondes) de l'heure du serveur pour empêcher les attaques par rejeu. La fenêtre d'horodatage est configurable mais par défaut à 5 minutes.

**Note** : Actuellement, l'en-tête `X-Key-Id` pour la rotation des clés n'est pas pris en charge. Cette fonctionnalité est prévue pour les versions futures.

## Points de Terminaison

### Vérification de Santé

**GET /healthz**

Vérifier l'état de santé du service.

**Réponse :**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Créer un Défi

**POST /v1/otp/challenges**

Créer un nouveau défi de vérification et envoyer un code de vérification.

**Requête :**
```json
{
  "user_id": "u_123",
  "channel": "sms",
  "destination": "+8613800138000",
  "purpose": "login",
  "locale": "zh-CN",
  "client_ip": "192.168.1.1",
  "ua": "Mozilla/5.0..."
}
```

**Réponse :**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

**Réponses d'Erreur :**

Toutes les réponses d'erreur suivent ce format :
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "message d'erreur optionnel"
}
```

Codes d'erreur possibles :
- `invalid_request` : Échec de l'analyse du corps de la requête
- `user_id_required` : Champ requis `user_id` manquant
- `invalid_channel` : Type de canal invalide (doit être "sms" ou "email")
- `destination_required` : Champ requis `destination` manquant
- `rate_limit_exceeded` : Limite de débit dépassée
- `resend_cooldown` : Période de réenvoi en attente non expirée
- `user_locked` : L'utilisateur est temporairement verrouillé
- `internal_error` : Erreur interne du serveur

Codes de Statut HTTP :
- `400 Bad Request` : Paramètres de requête invalides
- `401 Unauthorized` : Échec de l'authentification
- `403 Forbidden` : Utilisateur verrouillé
- `429 Too Many Requests` : Limite de débit dépassée
- `500 Internal Server Error` : Erreur interne du serveur

### Vérifier un Défi

**POST /v1/otp/verifications**

Vérifier un code de défi.

**Requête :**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**Réponse (Succès) :**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**Réponse (Échec) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Réponses d'Erreur :**

Codes d'erreur possibles :
- `invalid_request` : Échec de l'analyse du corps de la requête
- `challenge_id_required` : Champ requis `challenge_id` manquant
- `code_required` : Champ requis `code` manquant
- `invalid_code_format` : Le format du code de vérification est invalide
- `expired` : Le défi a expiré
- `invalid` : Code de vérification invalide
- `locked` : Défi verrouillé en raison de trop de tentatives
- `verification_failed` : Échec général de la vérification
- `internal_error` : Erreur interne du serveur

Codes de Statut HTTP :
- `400 Bad Request` : Paramètres de requête invalides
- `401 Unauthorized` : Échec de la vérification
- `403 Forbidden` : Utilisateur verrouillé
- `500 Internal Server Error` : Erreur interne du serveur

### Révoquer un Défi

**POST /v1/otp/challenges/{id}/revoke**

Révoquer un défi (optionnel).

**Réponse (Succès) :**
```json
{
  "ok": true
}
```

**Réponse (Échec) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Réponses d'Erreur :**

Codes d'erreur possibles :
- `challenge_id_required` : ID de défi manquant dans le paramètre URL
- `internal_error` : Erreur interne du serveur

Codes de Statut HTTP :
- `400 Bad Request` : Requête invalide
- `500 Internal Server Error` : Erreur interne du serveur

## Limitation du Débit

Herald implémente une limitation du débit multidimensionnelle :

- **Par Utilisateur** : 10 requêtes par heure (configurable)
- **Par IP** : 5 requêtes par minute (configurable)
- **Par Destination** : 10 requêtes par heure (configurable)
- **Délai de Réenvoi** : 60 secondes entre les réenvois

## Codes d'Erreur

Cette section liste tous les codes d'erreur possibles retournés par l'API.

### Erreurs de Validation de Requête
- `invalid_request` : Échec de l'analyse du corps de la requête ou JSON invalide
- `user_id_required` : Champ requis `user_id` manquant
- `invalid_channel` : Type de canal invalide (doit être "sms" ou "email")
- `destination_required` : Champ requis `destination` manquant
- `challenge_id_required` : Champ requis `challenge_id` manquant
- `code_required` : Champ requis `code` manquant
- `invalid_code_format` : Le format du code de vérification est invalide

### Erreurs d'Authentification
- `authentication_required` : Aucune authentification valide fournie
- `invalid_timestamp` : Format d'horodatage invalide
- `timestamp_expired` : L'horodatage est en dehors de la fenêtre autorisée (5 minutes)
- `invalid_signature` : Échec de la vérification de la signature HMAC

### Erreurs de Défi
- `expired` : Le défi a expiré
- `invalid` : Code de vérification invalide
- `locked` : Défi verrouillé en raison de trop de tentatives
- `too_many_attempts` : Trop de tentatives échouées (peut être inclus dans `locked`)
- `verification_failed` : Échec général de la vérification

### Erreurs de Limitation du Débit
- `rate_limit_exceeded` : Limite de débit dépassée
- `resend_cooldown` : Période de réenvoi en attente non expirée

### Erreurs de Statut Utilisateur
- `user_locked` : L'utilisateur est temporairement verrouillé

### Erreurs Système
- `internal_error` : Erreur interne du serveur
