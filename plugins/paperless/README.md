# Paperless-ngx

`com.phoedox.paperless` · version 0.1.0

Finds documents in Paperless-ngx for AI clients: a full-text search, and the invoices, manuals and warranty papers of an entry (searched by its serial number, model and name). Documents stay in Paperless-ngx; nothing is imported or changed, and the tools return title, correspondent, document type, creation date, tags and a link to the document, not its content.

## What it adds

- **Tools for AI clients (MCP):**
  - `paperless.search`: Full-text search in Paperless-ngx. Returns documents with title, correspondent, document type, creation date, tags and a link to the document in Paperless-ngx.
  - `paperless.for_entry`: Paperless-ngx documents about one entry (invoices, manuals, warranty papers), found by its serial number, model and name.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `token`.

**Settings:** url (the address of Paperless-ngx, e.g. https://paperless.example.org) and token (the API auth token of a Paperless-ngx user; results are limited to the documents this user may see, so a user with view rights only is enough). Reads entries only to build the search for paperless.for_entry.

## Permissions

- `graph:read`: reads the entries of the project.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/paperless/...
```

```bash
scripts/pack.sh paperless
```

The package lands in `dist/com.phoedox.paperless-0.1.0.phxp`. See the repository README for the layout of a plugin.
