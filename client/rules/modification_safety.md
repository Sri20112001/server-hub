# Modification Safety

When editing existing code:

* preserve existing behavior unless explicitly changing it
* avoid unrelated refactors
* avoid rewriting entire files unnecessarily
* change the smallest possible surface area
* preserve public APIs when possible
* avoid introducing breaking changes
* keep existing naming conventions consistent

Before modifying:

* identify what depends on the code
* identify possible side effects
* preserve backward compatibility where reasonable
