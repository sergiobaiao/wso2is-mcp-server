# Contract: Self-Service Additions (`self-service-mcp`)

The 28 existing tools in `docs/self-service-mcp.md` keep names and behavior. New tools use the
same `newTool` framework (annotations, confirmation sentence, redaction, elicitation).

| Tool | Kind | IS operation | Notes |
|---|---|---|---|
| `list_my_approval_tasks` | read | `GET /api/users/v2/me/approval-tasks` | Filter by `status` |
| `get_my_approval_task` | read | `GET /api/users/v2/me/approval-tasks/{task-id}` | |
| `decide_approval_task` | write | `PUT /api/users/v2/me/approval-tasks/{task-id}/state` | `action` ∈ `APPROVE`, `REJECT`, `CLAIM`, `RELEASE` |
| `list_my_organizations` | read | `GET /api/users/v1/me/organizations` | Also `root`, `root/descendants` via `scope` input |
| `export_my_data` | read | `GET /api/identity/user/v1.0/pi-info` | Not a flow operation (research.md R10); read-only export of the user's own data |
| `list_consent_records` | read | `GET /api/users/v1/me/consents` | User consent API (7.3) |
| `get_consent_record` | read | `GET /api/users/v1/me/consents/{consent-id}` | |
| `revoke_consent_record` | destructive | `POST /api/users/v1/me/consents/{consent-id}/revoke` | |

Authorized apps v2: `list_authorized_apps` and `revoke_app_access` switch to
`/api/users/v2/me/authorized-apps` when available and fall back to v1 on `404`.

Excluded by default (flow): `POST /api/users/v1/me/change-password`, `DELETE /scim2/Me`,
`/api/identity/user/v1.0/me/*` code endpoints.
