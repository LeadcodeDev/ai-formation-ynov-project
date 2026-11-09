# Glossaire

| Terme | Définition | Synonyme rejeté |
|---|---|---|
| Equipment model (modèle) | Type de matériel : nom, catégorie, durée max de prêt | equipment |
| Item (exemplaire) | Unité physique d'un modèle, identifiée par un numéro d'inventaire | device, asset |
| Loan (emprunt) | Prêt d'un exemplaire à un emprunteur | booking, reservation |
| Borrower (emprunteur) | Titulaire du claim `sub` du JWT, sans table `users` | user |
| Actor (acteur) | Identité authentifiée qui exécute un cas d'usage | caller |
| Active loan | Emprunt sans date de retour effective | open loan |
| Overdue loan | Emprunt actif dont `now` dépasse strictement `due_at` | late loan |
| Due at (`due_at`) | Minuit Europe/Paris qui clôt le dernier jour autorisé | deadline |
| Extension (prolongation) | Report unique de `due_at` d'une durée max de prêt | renewal |
