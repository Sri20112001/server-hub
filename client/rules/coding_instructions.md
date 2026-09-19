# React Component & Frontend Coding Guidelines

# Core Principles

* Prefer simple and maintainable solutions
* Keep components loosely coupled
* Prioritize readability over clever abstractions
* Follow existing project architecture and patterns
* Optimize only when necessary

---

# General Rules

* Use TypeScript
* Use ES6 modules
* Use arrow function components only
* Prefer composition over large monolithic components

---

# Project Architecture

* Follow existing architecture patterns
* Prefer feature-based structure for scalability
* Avoid deeply nested folders
* Keep related files close together

Example:

```txt id="j3d3h9"
src/
  api/
  app/Router/
  components/
    common/
    DashComp/
    dev/
    forms/
    layout/
    modal/
    ui/
  data/
  hooks/
  icons/
  pages/
  services/
  store/
  styles/
  utils/
  types/api/
  validation/
  workers/
```

---

# Component Design

* Keep components focused on a single responsibility
* Separate UI and business logic clearly
* Move reusable or complex logic into custom hooks
* Keep components extendable through props
* Avoid tightly coupled components

---

# Styling

* Follow the existing styling approach
* Prefer `module.css` unless the project already uses another system
* Keep styles modular and component-scoped
* Avoid global style leakage
* Avoid inline styles unless necessary

---

# State Management

* Keep state minimal
* Avoid storing derived state
* Compute values when possible
* Keep state close to where it is used
* Lift state only when necessary

Avoid:

```js id="ojjlwm"
const [fullName, setFullName] = useState('')
```

Prefer:

```js id="7vq6hy"
const fullName = `${firstName} ${lastName}`
```

---

# Performance

* Avoid unnecessary re-renders
* Use `useMemo` and `useCallback` only when beneficial
* Avoid inline object/function creation in performance-sensitive areas
* Memoize expensive computations only when needed
* Prefer simpler implementations before optimization

---

# Reusability

* Make components configurable via props
* Avoid hardcoded values
* Reuse shared logic through hooks/utilities/services
* Extract repeated UI patterns into reusable components only when duplication becomes meaningful

Avoid premature abstraction.

---

# Data Rendering

* Use arrays + `map()` for repeated UI
* Keep data separate from rendering logic
* Use stable unique keys
* Prefer database IDs or UUIDs

Avoid:

```jsx id="o8jlwm"
{items.map((item, index) => (
  <Card key={index} />
))}
```

Prefer:

```jsx id="b9jlwm"
{items.map((item) => (
  <Card key={item.id} />
))}
```

---

# Event Handling

* Define handlers separately
* Use clear naming conventions

Examples:

```js id="jqjlwm"
handleClick
handleSubmit
handleChange
handleDelete
```

Avoid inline handlers in performance-sensitive lists or large renders.

---

# API Handling

* Do not place API calls directly inside UI components
* Use services or custom hooks for API logic
* Keep networking concerns isolated

Example:

```txt id="z7jlwm"
api/        # API layer modules
services/   # notificationService and other services
hooks/      # custom hooks consuming API layer
components/ # UI only
```

---

# Forms

* Use controlled components
* Validate user input properly
* Show validation feedback clearly
* Prevent invalid submissions when possible

---

# Side Effects

* Use `useEffect` only for actual side effects
* Manage dependencies correctly
* Clean up subscriptions, listeners, and timers

Always clean up:

* intervals
* event listeners
* websocket connections
* subscriptions

---

# Accessibility

* Use semantic HTML
* Associate labels with inputs
* Ensure keyboard accessibility
* Add ARIA attributes where necessary
* Ensure focus visibility
* Avoid inaccessible click-only interactions

Prefer:

```jsx id="lqjlwm"
<button type="button">
```

Over:

```jsx id="x5jlwm"
<div onClick={...}>
```

---

# Error Handling

Always handle:

* loading state
* success state
* empty state
* error state

Avoid blank screens.

Provide meaningful fallback UI.

---

# Naming Conventions

## Components

Use PascalCase:

```txt id="7gjlwm"
UserCard
ProductModal
ThemePreview
```

---

## Variables

Use camelCase:

```js id="k2jlwm"
userData
isLoading
themeConfig
```

---

## Hooks

Hooks must start with `use`:

```js id="g4jlwm"
useAuth
useTheme
useFetchUsers
```

---

## Loading States

Loading booleans should:

* start with `is`
* default to `false`

Example:

```js id="r1jlwm"
const [isLoading, setIsLoading] =
  useState(false)
```

---

# Props

* Destructure props clearly
* Provide defaults when appropriate
* Keep prop interfaces small and understandable
* Avoid excessive prop drilling

Prefer:

```jsx id="t9jlwm"
function Button({
  variant = 'primary',
  children,
}) {}
```

---

# JSX Structure

* Avoid unnecessary wrapper elements
* Keep JSX readable
* Split large JSX blocks into smaller components when needed

Prefer readability over compactness.

---

# Comments

* Add short and meaningful comments
* Explain intent, not obvious syntax
* Avoid redundant comments

Avoid:

```js id="f3jlwm"
// Increment count
count++
```

Prefer:

```js id="h8jlwm"
// Prevent duplicate submissions
```

---

# Scalability Guidelines

* Keep components loosely coupled
* Minimize cross-component dependencies
* Prefer modular architecture
* Design for incremental extension
* Avoid overengineering early implementations

Introduce abstractions only when:

* duplication becomes meaningful
* scaling requirements are clear
* flexibility is genuinely needed

---

# File Naming Conventions

## Components

```txt id="v2jlwm"
UserCard.jsx
ThemeEditor.jsx
```

---

## Hooks

```txt id="c5jlwm"
useTheme.js
useAuth.js
```

---

## Styles

```txt id="n6jlwm"
Button.module.css
Navbar.module.css
```

---

# Recommended Folder Structure

```txt id="d7jlwm"
src/
  api/                  # API layer (auth, course, user, dashboard, etc.)
  app/Router/           # AuthGuard, Route, FallbackSpinner
  components/
    common/             # Shared UI primitives (Header, Sidebar, SearchBar, etc.)
    DashComp/           # Dashboard-specific components (charts, stats, tables)
    dev/                # DevThemeSwitcher (dev only)
    forms/              # QuickAddForm, UserFormTable, UserRowForm
    layout/             # RoleGuard
    modal/              # QuickReport, SelectUsersModal
    ui/                 # Reusable UI components (DataTable, Modal, StatusChip, etc.)
  data/                 # Static constants & config data
  hooks/                # Custom hooks (useCSVImport, useFilter, usePaginatedCourses, etc.)
  icons/                # Custom SVG icon components
  pages/
  services/             # notificationService
  store/                # Zustand stores
  styles/
  utils/                # constants, sanitize, validation, notify, etc.
  validation/           # loginValidationSchema, userValidationSchema
  workers/              # csv.worker.js
```

---

# Final Engineering Principles

Always consider:

* readability
* maintainability
* scalability
* performance
* consistency with existing architecture

Prefer:

* incremental improvements
* reusable patterns
* clean separation of concerns

Avoid:

* unnecessary abstractions
* premature optimization
* tightly coupled systems
* large multi-purpose components
