# Future directions

## Additional adapters

Codex CLI, Gemini CLI, Aider and other agents can be added only after the adapter contract has been proven with Claude Code. Each adapter must state how it detects approvals, session restoration and structured changes.

## Collaboration

Shared read-only links, team roles and co-approval may be useful later, but would materially expand the security model. They are not implied by the single-owner architecture.

## Scheduling and task queues

Queued tasks can be added above session management after policies, resource limits and workspace isolation are designed. CAR MUST NOT run unattended tasks merely because a queue exists.

## Cluster support

Multiple homelab nodes require explicit placement, storage and failover design; the initial deployment is intentionally single-node.

