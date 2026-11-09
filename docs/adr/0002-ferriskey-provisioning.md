# ADR 0002 : IAM FerrisKey, provisioning par API

Statut : accepté

## Contexte

L'IAM de référence est FerrisKey. Le dépôt amont ne documente pas d'import de realm par fichier, seulement une API d'administration et un opérateur Kubernetes.

## Décision

Le realm est créé par un conteneur d'initialisation du compose qui appelle l'API d'administration (realm, client, rôles `student` `staff` `admin`, utilisateurs de test, mapper de rôles). L'API Go valide le JWT via JWKS : `iss`, `aud`, expiration. Le nom du claim de rôles est configurable.

## Alternative rejetée

Keycloak : realm importable par JSON, mais la décision utilisateur est FerrisKey.

## Conséquences

Le dossier `deploy/realm` contient un script de provisioning, pas un export de realm. Son comportement est vérifié en lançant la stack.
