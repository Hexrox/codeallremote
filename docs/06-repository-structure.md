# Repository structure

```text
.
├── adr/                  # Accepted architectural decisions
├── docs/                 # Product and technical specifications
├── tasks/                # Implementation-ready work packages
├── schemas/              # Versioned machine-readable contracts (future)
├── apps/
│   ├── server/           # CAR Core (future)
│   └── android/          # Primary client (future)
├── adapters/
│   └── claude-code/      # First agent adapter (future)
└── deploy/               # Homelab/VPS deployment assets (future)
```

Documentation is intentionally independent of a chosen programming language. Technology choices become ADRs before implementation.

