# Engineering Workflow Instructions

## Constraint Analysis

Before implementation, identify relevant constraints:

* performance-sensitive?
* SSR requirements?
* mobile-first requirements?
* accessibility requirements?
* bundle-size concerns?
* offline support?
* scalability expectations?

Consider these constraints before proposing architecture or abstractions.

---

# Step 0 — Existing Context Analysis

Identify:

* current architecture
* patterns already used
* state management style
* folder structure
* naming conventions
* reusable utilities/hooks
* performance constraints

Avoid introducing inconsistent patterns unless clearly justified.

Project folder structure:

```txt
src/
  api/                  # API layer (auth, course, user, dashboard, etc.)
  app/Router/           # AuthGuard, Route, FallbackSpinner
  components/
    common/             # Shared UI primitives
    DashComp/           # Dashboard-specific components
    dev/                # DevThemeSwitcher (dev only)
    forms/              # QuickAddForm, UserFormTable, UserRowForm
    layout/             # RoleGuard
    modal/              # QuickReport, SelectUsersModal
    ui/                 # Reusable UI components
  data/                 # Static constants & config data
  hooks/                # Custom hooks
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

# Step 1 — Understanding

Summarize the requirement in 2–3 concise lines.

Identify:

* core goal
* affected systems/components
* technical scope

---

# Step 2 — Clarification

If assumptions are reasonable:

* state assumptions clearly
* proceed without blocking unnecessarily

Proceed with assumptions when:

* implementation is reversible
* assumptions are low-risk
* conventions are obvious
* existing code patterns strongly imply intent

Ask questions only when assumptions affect:

* architecture
* APIs
* database structure
* scalability
* UX behavior
* security-sensitive logic

---

# Step 3 — Approaches

Provide multiple approaches only when meaningful trade-offs exist.

When comparing approaches, evaluate:

* complexity
* scalability
* maintainability
* performance
* developer experience
* implementation speed

Avoid unnecessary alternatives.

Prefer practical solutions over theoretical flexibility.

---

# Step 4 — Recommendation

Suggest:

* best production-grade approach
* simpler approach for quick implementation

Prefer simple implementations first.

Only introduce abstractions when:

* duplication appears
* scaling benefits are clear
* flexibility is genuinely needed

Avoid premature optimization and overengineering.

---

# Step 5 — Confirmation

Ask for confirmation only when:

* multiple valid architectures exist
* requirements are unclear
* implementation is large or non-trivial
* trade-offs significantly affect future scalability

Skip confirmation for:

* dependency installation
* small refactors
* obvious fixes
* routine implementations

Otherwise proceed directly.

---

# Step 6 — Execution

After confirmation (if needed), generate code.

Always:

* follow existing project architecture
* preserve consistency with current patterns
* prioritize reusability
* minimize unnecessary re-renders
* maintain separation of concerns
* follow performance best practices

React / TypeScript Rules:

* use ES modules
* use functional components
* use TypeScript strict typing
* use module.css
* avoid inline styles unless justified
* extract reusable logic into hooks/utilities
* avoid unnecessary abstractions

When improving existing code:

* avoid unnecessary rewrites
* preserve working logic when possible
* prefer incremental refactors
* minimize breaking changes

---

# Step 7 — Final Notes

Keep final notes brief.

Allow iteration without restarting the entire solution.

Include only:

* important trade-offs
* future improvements
* optimization opportunities
* known limitations

---

# General Guidelines

Always consider:

* existing project architecture
* reusability
* maintainability
* performance implications
* long-term scalability

Adjust response depth based on complexity:

* high complexity → full process
* low complexity → condensed flow

Apply this workflow primarily for:

* feature implementation
* component design
* architecture decisions
* system-level problems

Skip or compress steps for simple/direct queries.
