# Contract: SCIM Tools (administration server, curated)

Ported from `arazzo/is-scim2.arazzo.yaml` of `pabasara-mahindapala/wso2is-scim2-mcp-ai-gateway`
(21 workflows). Names, input field names, outputs and descriptions MUST match; no gateway is
involved. All accept optional `organization_id` (org SCIM paths `/o/scim2/...`) except
`list_agents`, `create_agent`, `delete_agent` (root only in 7.3).

Error convention for every tool: on failure the result is an MCP error using
tool-conventions.md; additionally the JSON fields `error` (SCIM `detail`) and `errorStatus`
(SCIM `status`) are returned for compatibility with the Arazzo outputs.

| Tool | Kind | Inputs | IS call(s) | Outputs |
|---|---|---|---|---|
| `list_users` | read | `count` | `GET /scim2/Users?attributes=<compact>&startIndex=1&count=` | `totalResults`, `users` |
| `search_users` | read | `filter`, `count` | `POST /scim2/Users/.search` (`SearchRequest`, compact attributes) | `totalResults`, `users` |
| `get_user` | read | `userId` | `GET /scim2/Users/{id}` | `user` |
| `create_user` | write | `userName`, `password`, `givenName`, `familyName`, `email`; aliases `username`, `first_name`, `last_name`, `userstore_domain` (default `PRIMARY`) | `POST /scim2/Users` | `id`, `userName` |
| `update_user_attribute` | write | `userId`, `attribute`, `value` | `PATCH /scim2/Users/{id}` replace | `id`, `userName` |
| `update_user_profile` | write | `userId` + one field per `SCIM_PROFILE_ATTRIBUTES` entry (short names: last path segment; legacy names `region`, `department`, `employeeNumber`, `costCenter`, `customerTier`, `contractEndDate` accepted when configured) | `PATCH /scim2/Users/{id}` | `id`, `userName`, `addresses`, `enterprise`, `custom` |
| `delete_user` | destructive | `userId` | `DELETE /scim2/Users/{id}` | `status` |
| `list_groups` | read | `count` | `GET /scim2/Groups?attributes=displayName,members` | `totalResults`, `groups` |
| `create_group` | write | `displayName` | `POST /scim2/Groups` | `id`, `displayName` |
| `add_user_to_group` | write | `groupId`, `userId` | `GET /scim2/Users/{id}?attributes=userName`, then `PATCH /scim2/Groups/{id}` add member | `group`, `addedUser` |
| `remove_user_from_group` | destructive | `groupId`, `userId` | `PATCH /scim2/Groups/{id}` remove `members[value eq "<userId>"]` | `group` |
| `delete_group` | destructive | `groupId` | `DELETE /scim2/Groups/{id}` | `status` |
| `list_roles` | read | `count` | `GET /scim2/v2/Roles?startIndex=1&count=` | `totalResults`, `roles` |
| `create_role` | write | `displayName` | `POST /scim2/v2/Roles` (organization audience) | `id`, `displayName` |
| `assign_role_to_user` | write | `roleId`, `userId` | `PATCH /scim2/v2/Roles/{id}` (`add`, path `users`, value `[{"value": userId}]`) | `role` |
| `remove_role_from_user` | destructive | `roleId`, `userId` | `PATCH /scim2/v2/Roles/{id}` (`remove`, path `users[value eq "<userId>"]`) | `role` |
| `delete_role` | destructive | `roleId` | `DELETE /scim2/v2/Roles/{id}` | `status` |
| `list_agents` | read | `count` | `GET /scim2/Agents?startIndex=1&count=` | `totalResults`, `agents` |
| `create_agent` | write | `displayName`, `description` | `POST /scim2/Agents` | `id`, `userName` (agent credentials redacted) |
| `delete_agent` | destructive | `agentId` | `DELETE /scim2/Agents/{id}` | `status` |
| `bulk_create_users` | write | `users` (array of `{userName, password, givenName, familyName, email?}`, 1–100); legacy `user1*`/`user2*` fields accepted | `POST /scim2/Bulk` | `results` (per user: `location`, `status`) |

Compact user attributes: `userName,name,emails,addresses,` the enterprise extension, and the
schemas of `SCIM_PROFILE_ATTRIBUTES`.

`create_user` merges the existing admin tool (`username`, `password`, `email`, `first_name`,
`last_name`, `userstore_domain`) with the SCIM workflow inputs; both spellings are accepted, and
giving both spellings of one field with different values is an input error. `password` is never
echoed.

Roles v2 in 7.3 exposes only `PATCH /v2/Roles/{id}` for membership changes (no `/Users`
sub-resource); user, group and permission changes are patch operations on paths `users`,
`groups` and `permissions`.

## Additional curated SCIM tools (gaps)

| Tool | Kind | Purpose |
|---|---|---|
| `patch_user` | write | Raw SCIM PATCH `Operations` on a user |
| `get_group` | read | Group with members |
| `update_group` | write | Rename a group |
| `get_role` | read | Role with users, groups, permissions, audience |
| `assign_role_to_group` | write | Add group to role |
| `remove_role_from_group` | destructive | Remove group from role |
| `update_role_permissions` | write | Add/remove API scopes (`permissions`) of a role |
| `get_agent` | read | One agent |
