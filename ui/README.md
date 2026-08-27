# CodeKanban UI

Vue 3 frontend for CodeKanban. The production bundle is embedded by the Go application.

## Setup

```sh
pnpm install
```

## Commands

```sh
# Development server
pnpm dev

# Type-check and production build
pnpm build

# Run Oxlint followed by ESLint without modifying files
pnpm lint

# Apply safe lint fixes
pnpm lint:fix

# Run type-checking, linting, and tests
pnpm check
```

The development proxy reads the backend address from `../config.yaml`, then
`../data/config.yaml`. It prefers `domain`, falls back to `serveAt`, and finally uses
`http://127.0.0.1:3007`.
