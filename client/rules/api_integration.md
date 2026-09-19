# API Integration Rules

## API Structure

* Keep all API calls in separate service/API files.
* Do not write API requests directly inside components.
* Group related APIs by feature/module.
* Reuse existing API utilities and patterns.

Example:

* api/userApi.ts
* api/courseApi.ts
* api/dashboardApi.ts

---

## Reusable Fetching Hook

For data fetching, create reusable custom hooks instead of duplicating fetch logic.

The hook should handle:

* loading state
* error state
* success state
* refetch functionality
* cleanup when needed

Example:

* useApi()
* useFetch()
* useQueryData()

Components should consume reusable hooks instead of implementing fetch logic directly.

---

## Loading & Error Handling

Every API integration must handle:

* loading state
* error state
* success state
* empty state when applicable

Never leave API calls without proper error handling.

Provide user-friendly error messages.

Avoid silent failures.

---

## Component Rules

Components should:

* focus on UI rendering
* avoid direct API logic
* avoid duplicated fetch logic
* consume hooks/services cleanly

Keep components small and maintainable.

---

## API Rules

Always:

* use async/await
* handle try/catch properly
* validate API responses
* use TypeScript typing
* centralize API configuration
* use environment variables for secrets
* avoid hardcoded URLs/tokens

---

## Performance

* avoid unnecessary API calls
* prevent duplicate requests
* cache/reuse data when appropriate
* minimize unnecessary re-renders

---

## Code Quality

Prefer:

* simple implementations
* reusable logic
* consistent naming
* scalable folder structure
* incremental improvements

Avoid:

* overengineering
* unnecessary abstractions
* duplicated fetch logic
* API logic inside components

---

# Folder Reference

```txt
src/
  api/        # API layer — all API call modules go here
  hooks/      # custom hooks consuming API layer
  services/   # notificationService and other non-API services
  store/      # Zustand stores
  components/ # UI only, no API logic
```