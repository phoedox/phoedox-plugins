# GitLab

`com.phoedox.gitlab` · version 0.1.0

Imports the projects of a GitLab group (with subgroups) into the knowledge graph (visibility, default branch, topics, last activity) and lists open issues and merge requests for AI clients. Works with gitlab.com and self-managed GitLab.

## What it adds

- **Kinds of entries:** `gitlab_project`.
- **Tools for AI clients (MCP):**
  - `gitlab.sync`: Import the projects of the configured group now.
  - `gitlab.open`: Open issues and merge requests of one project of the configured group.
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `group`, `token`.

**Settings:** url (e.g. https://gitlab.com), group (path, e.g. acme/infra), optional token (personal or group access token with read_api; needed for private projects).

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:gitlab_project`: creates and updates entries of the kinds `gitlab_project`.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:group`: stores the setting `group`, encrypted with the project.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/gitlab/...
```

```bash
scripts/pack.sh gitlab
```

The package lands in `dist/com.phoedox.gitlab-0.1.0.phxp`. See the repository README for the layout of a plugin.
