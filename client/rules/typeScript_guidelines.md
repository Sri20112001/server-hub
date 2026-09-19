# TypeScript Guidelines

* Prefer explicit typing for public APIs
* Avoid `any` unless absolutely necessary
* Prefer inference for simple local values
* Use discriminated unions for variant-based logic
* Keep types close to usage unless shared
* Avoid over-complicated generic abstractions
* Prefer readable types over highly clever types
* Use `import type` for type-only imports (`verbatimModuleSyntax` is enabled)
* Do not use `enum` or `namespace` — `erasableSyntaxOnly` is enforced
* Prefer `const` object maps or union types over enums
* Clean up unused locals and parameters — `noUnusedLocals` and `noUnusedParameters` are enabled

For scalable systems:

* separate domain types from UI types
* prefer schema-driven typing when possible
* keep runtime validation aligned with TypeScript types
