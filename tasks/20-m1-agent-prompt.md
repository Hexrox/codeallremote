# M1 implementation prompt

Use this prompt when delegating a task to an implementation agent:

```text
You are implementing one CAR task only.

Read:
- the selected task file;
- every document listed under Inputs;
- applicable ADRs;
- docs/43-implementation-guidelines.md;
- docs/45-definition-of-ready-done.md.

Constraints:
- do not expand scope;
- do not change protocol, security boundaries or persistence semantics silently;
- write tests for acceptance criteria and failure paths;
- do not use real provider credentials or public network in tests;
- do not log secrets, prompts or private workspace content.

Before finishing, provide the handoff report from docs/44-agent-workflow.md and fill tasks/19-m1-review-template.md.
```

