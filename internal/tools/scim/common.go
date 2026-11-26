/*
 * Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package scim implements the 29 curated SCIM 2.0 administration tools
// (21 ported Arazzo workflows and 8 gap tools) backed by the IS catalog and client.
package scim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
)

const (
	enterpriseUserSchema = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	customUserSchema     = "urn:scim:schemas:extension:custom:User"
	coreUserSchema       = "urn:ietf:params:scim:schemas:core:2.0:User"
	coreGroupSchema      = "urn:ietf:params:scim:schemas:core:2.0:Group"
	roleV2Schema         = "urn:ietf:params:scim:schemas:extension:2.0:Role"
	patchOpSchema        = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	searchReqSchema      = "urn:ietf:params:scim:api:messages:2.0:SearchRequest"
	bulkReqSchema        = "urn:ietf:params:scim:api:messages:2.0:BulkRequest"
	defaultListCount     = 100
)

type opPair struct {
	root *catalog.Operation
	org  *catalog.Operation
}

type opSet struct {
	listUsers   opPair
	searchUsers opPair
	getUser     opPair
	createUser  opPair
	patchUser   opPair
	deleteUser  opPair
	bulkUsers   opPair
	listGroups  opPair
	createGroup opPair
	getGroup    opPair
	patchGroup  opPair
	deleteGroup opPair
	listRoles   opPair
	createRole  opPair
	getRole     opPair
	patchRole   opPair
	deleteRole  opPair
	listAgents  *catalog.Operation
	createAgent *catalog.Operation
	getAgent    *catalog.Operation
	deleteAgent *catalog.Operation
}

// Tools returns all 29 curated SCIM tool definitions, failing if any referenced catalog operation is missing.
func Tools(cat *catalog.Catalog, cl *client.Client, cfg *config.Config) ([]toolkit.ToolDef, error) {
	if cat == nil {
		return nil, errors.New("catalog is required")
	}
	if cl == nil {
		return nil, errors.New("client is required")
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	ops, err := loadOpSet(cat)
	if err != nil {
		return nil, err
	}

	var defs []toolkit.ToolDef
	defs = append(defs, userTools(ops, cl, cfg)...)
	defs = append(defs, groupTools(ops, cl)...)
	defs = append(defs, roleTools(ops, cl)...)
	defs = append(defs, agentTools(ops, cl)...)
	defs = append(defs, bulkTools(ops, cl)...)
	return defs, nil
}

// Register validates catalog operations and registers the curated SCIM tools into reg.
func Register(reg *toolkit.Registry, cat *catalog.Catalog, cl *client.Client, cfg *config.Config) error {
	if reg == nil {
		return errors.New("registry is required")
	}
	defs, err := Tools(cat, cl, cfg)
	if err != nil {
		return err
	}
	for _, def := range defs {
		if err := reg.Register(def); err != nil {
			return fmt.Errorf("register SCIM tool %q: %w", def.Name, err)
		}
	}
	return nil
}

func loadOpSet(cat *catalog.Catalog) (*opSet, error) {
	var err error
	s := &opSet{}

	pairs := []struct {
		dst    *opPair
		rootID string
		orgID  string
	}{
		{&s.listUsers, "users.getUser", "users.org.getUser"},
		{&s.searchUsers, "users.getUsersByPost", "users.org.getUsersByPost"},
		{&s.getUser, "users.getUserById", "users.org.getUserById"},
		{&s.createUser, "users.createUser", "users.org.createUser"},
		{&s.patchUser, "users.patchUser", "users.org.patchUser"},
		{&s.deleteUser, "users.deleteUser", "users.org.deleteUser"},
		{&s.bulkUsers, "users.createBulkUsers", "users.org.createBulkUsers"},
		{&s.listGroups, "groups.getGroup", "groups.org.getGroup"},
		{&s.createGroup, "groups.createGroup", "groups.org.createGroup"},
		{&s.getGroup, "groups.getGroupById", "groups.org.getGroupById"},
		{&s.patchGroup, "groups.patchGroup", "groups.org.patchGroup"},
		{&s.deleteGroup, "groups.deleteGroup", "groups.org.deleteGroup"},
		{&s.listRoles, "roles.getRoleV2", "roles.org.getRoleV2"},
		{&s.createRole, "roles.createRoleV2", "roles.org.createRoleV2"},
		{&s.getRole, "roles.getRolebyIdV2", "roles.org.getRolebyIdV2"},
		{&s.patchRole, "roles.patchScim2V2RolesById", "roles.org.patchRole"},
		{&s.deleteRole, "roles.deleteRoleV2", "roles.org.deleteRoleV2"},
	}

	for _, p := range pairs {
		*p.dst, err = lookupPair(cat, p.rootID, p.orgID)
		if err != nil {
			return nil, err
		}
	}

	singles := []struct {
		dst **catalog.Operation
		id  string
	}{
		{&s.listAgents, "agents.getAgents"},
		{&s.createAgent, "agents.createAgent"},
		{&s.getAgent, "agents.getAgentById"},
		{&s.deleteAgent, "agents.deleteAgentById"},
	}
	for _, single := range singles {
		*single.dst, err = lookupOp(cat, single.id)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func lookupPair(cat *catalog.Catalog, rootID, orgID string) (opPair, error) {
	rootOp, err := lookupOp(cat, rootID)
	if err != nil {
		return opPair{}, err
	}
	orgOp, err := lookupOp(cat, orgID)
	if err != nil {
		return opPair{}, err
	}
	return opPair{root: rootOp, org: orgOp}, nil
}

func lookupOp(cat *catalog.Catalog, id string) (*catalog.Operation, error) {
	op, ok := cat.Get(id)
	if !ok || op == nil {
		return nil, fmt.Errorf("required SCIM catalog operation %q not found", id)
	}
	return op, nil
}

func (p opPair) selectOp(ctx context.Context, args map[string]any) (*catalog.Operation, context.Context, error) {
	if raw, exists := args["organization_id"]; exists && raw != nil {
		orgStr, ok := raw.(string)
		if !ok {
			return nil, ctx, errors.New("invalid input: organization_id: must be a string")
		}
		if trimmed := strings.TrimSpace(orgStr); trimmed != "" {
			return p.org, client.WithOrganizationID(ctx, trimmed), nil
		}
	}
	if cc, ok := client.CredentialContextFrom(ctx); ok && strings.TrimSpace(cc.OrganizationID) != "" {
		return p.org, ctx, nil
	}
	return p.root, ctx, nil
}

func rejectOrgID(ctx context.Context, op *catalog.Operation, args map[string]any) error {
	if raw, exists := args["organization_id"]; exists && raw != nil {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) != "" {
			return fmt.Errorf("operation %s is not available for organizations", op.ID)
		}
	}
	if cc, ok := client.CredentialContextFrom(ctx); ok && strings.TrimSpace(cc.OrganizationID) != "" {
		return fmt.Errorf("operation %s is not available for organizations", op.ID)
	}
	return nil
}

// withoutBodySchema returns a shallow copy of op without the OpenAPI body schema so
// endpoints whose vendored specs contain malformed schema definitions (e.g. schemas as object)
// still pass through client.Do's parameter validation, auth/org-switch, scope wiring, and error handling.
func withoutBodySchema(op *catalog.Operation) *catalog.Operation {
	return toolkit.WithoutBodySchema(op)
}

func doOp(ctx context.Context, cl *client.Client, op *catalog.Operation, pathParams map[string]string, query map[string]any, body any) (any, error) {
	res, err := cl.Do(ctx, op, pathParams, query, body)
	if err != nil {
		return nil, mapSCIMError(err, op)
	}
	return res, nil
}

func doOpBodylessValidation(ctx context.Context, cl *client.Client, op *catalog.Operation, pathParams map[string]string, query map[string]any, body any) (any, error) {
	res, err := cl.Do(ctx, withoutBodySchema(op), pathParams, query, body)
	if err != nil {
		return nil, mapSCIMError(err, op)
	}
	return res, nil
}

func mapSCIMError(err error, op *catalog.Operation) error {
	if err == nil {
		return nil
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	actionable := client.FormatError(apiErr, op)
	detail, status := extractSCIMErrorFields(apiErr)

	payload, marshalErr := json.Marshal(map[string]any{
		"error":       detail,
		"errorStatus": status,
	})
	if marshalErr != nil {
		return errors.New(actionable)
	}
	return fmt.Errorf("%s\n%s", actionable, string(payload))
}

func extractSCIMErrorFields(apiErr *client.APIError) (detail string, status string) {
	status = strconv.Itoa(apiErr.StatusCode)
	detail = strings.TrimSpace(apiErr.Message)

	if len(apiErr.Body) > 0 {
		var obj map[string]any
		if json.Unmarshal(apiErr.Body, &obj) == nil {
			if d, ok := obj["detail"].(string); ok && strings.TrimSpace(d) != "" {
				detail = strings.TrimSpace(d)
			}
			switch s := obj["status"].(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					status = strings.TrimSpace(s)
				}
			case float64:
				status = strconv.Itoa(int(s))
			}
		}
	}

	if detail == "" {
		detail = http.StatusText(apiErr.StatusCode)
	}
	return detail, status
}

func unionScopes(ops ...*catalog.Operation) []string {
	set := make(map[string]bool)
	for _, op := range ops {
		if op == nil {
			continue
		}
		for _, s := range op.Scopes {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				set[trimmed] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func requiredString(args map[string]any, name string) (string, error) {
	raw, exists := args[name]
	if !exists || raw == nil {
		return "", fmt.Errorf("invalid input: %s: is required", name)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %s: must be a string", name)
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("invalid input: %s: must not be empty", name)
	}
	return trimmed, nil
}

func optionalString(args map[string]any, name string) (string, error) {
	raw, exists := args[name]
	if !exists || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %s: must be a string", name)
	}
	return strings.TrimSpace(s), nil
}

func optionalStringSlice(args map[string]any, name string) ([]string, error) {
	raw, exists := args[name]
	if !exists || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("invalid input: %s[%d]: must be a string", name, i)
			}
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid input: %s: must be an array of strings", name)
	}
}

func optionalCount(args map[string]any, def int) (int, error) {
	raw, exists := args["count"]
	if !exists || raw == nil {
		return def, nil
	}
	var count int
	switch v := raw.(type) {
	case int:
		count = v
	case int64:
		count = int(v)
	case float64:
		if v != float64(int(v)) {
			return 0, errors.New("invalid input: count: must be an integer")
		}
		count = int(v)
	default:
		return 0, errors.New("invalid input: count: must be an integer")
	}
	if count < 1 {
		return 0, errors.New("invalid input: count: must be at least 1")
	}
	return count, nil
}

func validateSCIMFilterValue(field, val string) error {
	if strings.ContainsAny(val, "\"\r\n") {
		return fmt.Errorf("invalid input: %s: contains invalid characters", field)
	}
	return nil
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok && s != nil {
		return s
	}
	return []any{}
}

func listOutput(resp any, listKey string) map[string]any {
	m := asMap(resp)
	items := asSlice(m["Resources"])
	total, hasTotal := m["totalResults"]
	if !hasTotal || total == nil {
		total = len(items)
	}
	return map[string]any{
		"totalResults": total,
		listKey:        items,
	}
}

type profileAttrSpec struct {
	raw         string
	shortName   string
	schemaURN   string
	attrName    string
	addressType string
}

var legacyAttributeSchemas = map[string]string{
	"department":      enterpriseUserSchema,
	"employeeNumber":  enterpriseUserSchema,
	"costCenter":      enterpriseUserSchema,
	"organization":    enterpriseUserSchema,
	"division":        enterpriseUserSchema,
	"region":          customUserSchema,
	"customerTier":    customUserSchema,
	"contractEndDate": customUserSchema,
}

func effectiveProfileAttrList(cfg *config.Config) []string {
	if cfg != nil && cfg.SCIMProfileAttributes != nil {
		return cfg.SCIMProfileAttributes
	}
	return strings.Split(config.DefaultSCIMProfileAttributes, ",")
}

func parseProfileAttributes(cfg *config.Config) []profileAttrSpec {
	rawList := effectiveProfileAttrList(cfg)
	seen := make(map[string]bool, len(rawList))
	specs := make([]profileAttrSpec, 0, len(rawList))

	for _, entry := range rawList {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		spec := parseOneProfileAttr(trimmed)
		if spec.shortName == "" || seen[spec.shortName] {
			continue
		}
		seen[spec.shortName] = true
		specs = append(specs, spec)
	}
	return specs
}

func parseOneProfileAttr(entry string) profileAttrSpec {
	if strings.HasPrefix(entry, "urn:") {
		idx := strings.LastIndex(entry, ":")
		if idx <= 0 || idx == len(entry)-1 {
			return profileAttrSpec{}
		}
		schemaURN := entry[:idx]
		attrName := entry[idx+1:]
		shortName := attrName
		if dot := strings.LastIndex(attrName, "."); dot != -1 && dot < len(attrName)-1 {
			shortName = attrName[dot+1:]
		}
		return profileAttrSpec{
			raw:       entry,
			shortName: shortName,
			schemaURN: schemaURN,
			attrName:  attrName,
		}
	}

	if strings.HasPrefix(entry, "addresses.") {
		sub := strings.TrimPrefix(entry, "addresses.")
		if sub != "" {
			return profileAttrSpec{
				raw:         entry,
				shortName:   sub,
				attrName:    entry,
				addressType: sub,
			}
		}
	}

	if dot := strings.LastIndex(entry, "."); dot != -1 && dot < len(entry)-1 {
		return profileAttrSpec{
			raw:       entry,
			shortName: entry[dot+1:],
			attrName:  entry,
		}
	}

	return profileAttrSpec{
		raw:       entry,
		shortName: entry,
		schemaURN: legacyAttributeSchemas[entry],
		attrName:  entry,
	}
}

func compactUserAttributes(cfg *config.Config) []string {
	attrs := []string{
		"userName",
		"name",
		"emails",
		"addresses",
		enterpriseUserSchema,
	}
	seen := make(map[string]bool, len(attrs)+4)
	for _, a := range attrs {
		seen[a] = true
	}
	for _, spec := range parseProfileAttributes(cfg) {
		if spec.schemaURN != "" && !seen[spec.schemaURN] {
			seen[spec.schemaURN] = true
			attrs = append(attrs, spec.schemaURN)
		}
	}
	return attrs
}
