# ADR 0001 : règle de retard

Statut : accepté

## Contexte

La spec dit : un emprunt est en retard si `now` dépasse la date prévue, évaluée à minuit Europe/Paris. La phrase admet deux lectures : minuit au début ou à la fin du jour prévu.

## Décision

Le dernier jour autorisé est la date calendaire Paris de `borrowed_at` plus `max_loan_days`. `due_at` vaut minuit Europe/Paris au terme de ce jour, soit 00:00 du lendemain. Un emprunt est en retard si et seulement si `now` est strictement après `due_at`. Une prolongation ajoute `max_loan_days` jours calendaires Paris à `due_at`, sans fixer d'heure en UTC.

## Alternative rejetée

Minuit au début du jour prévu : un rendu le jour dit à 10h serait en retard, ce qui contredit l'usage.

## Conséquences

`now == due_at` n'est pas en retard. Les changements d'heure (DST) sont absorbés par l'arithmétique de dates en `Europe/Paris`. Les bornes sont couvertes par des tests table-driven.
