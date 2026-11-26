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

// Package toolkit provides the shared MCP tool builder, approval elicitation,
// and tool registry used across curated, catalog, and per-operation tools.
package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/redact"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Layer identifies the registration layer of a tool.
type Layer string

const (
	LayerCurated   Layer = "curated"
	LayerCatalog   Layer = "catalog"
	LayerOperation Layer = "operation"
)

const (
	// DefaultMaxResponseBytes is the default response truncation ceiling when none is configured.
	DefaultMaxResponseBytes = config.DefaultMaxResponseBytes

	orgHintSentence = "Pass organization_id to run inside a sub-organization."
	confirmSentence = "Before calling, describe the exact change and its target to the user and get their explicit yes to it. The user's request is not a yes, and a yes to an earlier or different change doesn't carry over."
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ToolDef describes a single MCP tool to be normalized, annotated, and registered.
type ToolDef struct {
	Name        string
	Layer       Layer
	Kind        catalog.Kind
	Scopes      []string
	OrgCapable  bool
	Operation   *catalog.Operation
	SummaryFunc func(args map[string]any) string
	Description string
	Options     []mcp.ToolOption
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// ValidateName enforces the snake_case, <=64-character tool naming convention.
func ValidateName(name string) error {
	if !toolNamePattern.MatchString(name) {
		return fmt.Errorf("invalid tool name %q: must match ^[a-z][a-z0-9_]{0,63}$", name)
	}
	return nil
}

func validateKind(kind catalog.Kind) error {
	switch kind {
	case catalog.KindRead, catalog.KindWrite, catalog.KindDestructive:
		return nil
	default:
		return fmt.Errorf("invalid tool kind %q: must be read, write, or destructive", kind)
	}
}

// BuildTool validates def and builds an mcp-go ServerTool with standard annotations,
// description conventions, elicitation approval, error formatting, redaction, and truncation.
func BuildTool(def ToolDef, maxResponseBytes int) (server.ServerTool, error) {
	if err := ValidateName(def.Name); err != nil {
		return server.ServerTool{}, err
	}
	if err := validateKind(def.Kind); err != nil {
		return server.ServerTool{}, fmt.Errorf("tool %q: %w", def.Name, err)
	}
	if def.Handler == nil {
		return server.ServerTool{}, fmt.Errorf("tool %q: handler is required", def.Name)
	}

	maxBytes := maxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}

	desc := formatDescription(def)

	opts := make([]mcp.ToolOption, 0, len(def.Options)+5)
	opts = append(opts,
		mcp.WithDescription(desc),
		mcp.WithReadOnlyHintAnnotation(def.Kind == catalog.KindRead),
		mcp.WithDestructiveHintAnnotation(def.Kind == catalog.KindDestructive),
		mcp.WithOpenWorldHintAnnotation(true),
	)
	opts = append(opts, def.Options...)
	if def.OrgCapable {
		opts = append(opts, mcp.WithString(
			"organization_id",
			mcp.Description("Sub-organization ID to target instead of the root organization."),
		))
	}

	tool := mcp.NewTool(def.Name, opts...)

	return server.ServerTool{
		Tool: tool,
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			if args == nil {
				args = map[string]any{}
			}

			if orgID, ok := args["organization_id"].(string); ok {
				if trimmed := strings.TrimSpace(orgID); trimmed != "" {
					if !def.OrgCapable {
						// Never run a root-only tool in the root organization when the caller asked
						// for a sub-organization.
						target := def.Name
						if def.Operation != nil {
							target = def.Operation.ID
						}
						return mcp.NewToolResultError(fmt.Sprintf("operation %s is not available for organizations", target)), nil
					}
					ctx = client.WithOrganizationID(ctx, trimmed)
				}
			}

			if def.Kind != catalog.KindRead {
				summary := buildApprovalSummary(def, args)
				approved, _, err := RequestApproval(ctx, def.Name, summary, DefaultElicitationTimeout)
				if err != nil || !approved {
					return mcp.NewToolResultError(NotApprovedMessage), nil
				}
			}

			result, err := def.Handler(ctx, args)
			if err != nil {
				return mcp.NewToolResultError(client.FormatError(err, def.Operation)), nil
			}
			if result == nil {
				return mcp.NewToolResultText("Done."), nil
			}

			redacted := redact.Value(result)
			truncated := client.Truncate(redacted, maxBytes, def.Operation)
			encoded, err := json.MarshalIndent(truncated, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("encode tool response: %v", err)), nil
			}
			return mcp.NewToolResultText(string(encoded)), nil
		},
	}, nil
}

func formatDescription(def ToolDef) string {
	desc := strings.TrimSpace(def.Description)
	if def.OrgCapable && !strings.Contains(desc, orgHintSentence) {
		if desc != "" {
			desc += " "
		}
		desc += orgHintSentence
	}
	if (def.Kind == catalog.KindWrite || def.Kind == catalog.KindDestructive) && !strings.Contains(desc, confirmSentence) {
		if desc != "" {
			desc += " "
		}
		desc += confirmSentence
	}
	return desc
}

func buildApprovalSummary(def ToolDef, args map[string]any) string {
	var summary string
	if def.SummaryFunc != nil {
		summary = sanitizeSingleLine(def.SummaryFunc(args))
	}
	if summary != "" {
		return summary
	}

	var parts []string
	if def.Operation != nil && def.Operation.ID != "" {
		parts = append(parts, "operation="+def.Operation.ID)
	}
	if orgID, ok := args["organization_id"].(string); ok && strings.TrimSpace(orgID) != "" {
		parts = append(parts, "organization_id="+strings.TrimSpace(orgID))
	}

	targetKeys := []string{"id", "user_id", "group_id", "role_id", "app_id", "idp_id", "username", "userName", "name"}
	for _, k := range targetKeys {
		if v, ok := args[k]; ok && v != nil {
			s := sanitizeSingleLine(fmt.Sprint(v))
			if s != "" {
				parts = append(parts, k+"="+s)
			}
		}
	}

	if len(args) > 0 {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts = append(parts, "args=["+strings.Join(keys, ",")+"]")
	}

	if len(parts) == 0 {
		return fmt.Sprintf("execute %s operation", def.Kind)
	}
	return strings.Join(parts, " ")
}

func sanitizeSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
