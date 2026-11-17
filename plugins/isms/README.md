# ISMS controls

`com.phoedox.isms` · version 0.1.0

Helps to run an information security management system for ISO/IEC 27001 or BSI IT-Grundschutz in the knowledge graph. It adds the kind control (framework, reference, title, status, justification, owner, evidence, last review) and the relation covers from a control to any entry. You enter the references (e.g. A.5.15 or OPS.1.1.2.A3) and your own titles and texts; the plugin ships no catalogue and no texts of the standards. AI clients get the coverage (controls per framework and status, controls without evidence or without a review in the last 12 months, servers, VMs and services no control covers; isms.coverage) and the Statement of Applicability as Markdown (isms.soa). No settings and no network access; it only reads the project's entries.

## What it adds

- **Kinds of entries:** `control`.
- **Relations:** `covers`.
- **Tools for AI clients (MCP):**
  - `isms.coverage`: Controls per framework and status, controls without evidence, controls not reviewed in the last 12 months, and servers, VMs and services that no control covers.
  - `isms.soa`: Statement of Applicability as Markdown: every control with reference, title, applicability, status, justification, owner, evidence, last review and the entries it covers.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins**.

## Permissions

- `graph:read`: reads the entries of the project.

## Development

```bash
go test ./plugins/isms/...
```

```bash
scripts/pack.sh isms
```

The package lands in `dist/com.phoedox.isms-0.1.0.phxp`. See the repository README for the layout of a plugin.
