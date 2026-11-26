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

// Package workflows implements the curated workflows_approvals tools
// (contracts/admin-curated-tools.md).
package workflows

import (
	"context"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// defaultEngine is the built-in IS approval workflow engine.
const defaultEngine = "WorkflowEngine"

// Tools returns the four workflow tools.
func Tools(cat *catalog.Catalog, cl *client.Client, _ *config.Config) ([]toolkit.ToolDef, error) {
	ids := []string{
		"workflows_approvals.getWorkflows", "workflows_approvals.addWorkflow",
		"workflows_approvals.addWorkflowAssociation", "workflows_approvals.getWorkflowInstances",
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, id := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, id, ""); err != nil {
			return nil, err
		}
	}
	list, add, associate, instances := ops[0], ops[1], ops[2], ops[3]

	listOpts := []mcp.ToolOption{
		mcp.WithNumber("limit", mcp.Description("Maximum number of results.")),
		mcp.WithNumber("offset", mcp.Description("Number of results to skip.")),
		mcp.WithString("filter", mcp.Description("Filter expression.")),
	}
	listHandler := func(p toolkit.OpPair) func(context.Context, map[string]any) (any, error) {
		return func(ctx context.Context, args map[string]any) (any, error) {
			q, err := toolkit.Paging(args)
			if err != nil {
				return nil, err
			}
			f, err := toolkit.OptString(args, "filter")
			if err != nil {
				return nil, err
			}
			if f != "" {
				q["filter"] = f
			}
			return toolkit.Call(ctx, cl, p, nil, q, nil)
		}
	}

	return []toolkit.ToolDef{
		{
			Name: "list_workflows", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), Operation: list.Root,
			Description: "List the approval workflows defined in WSO2 Identity Server.",
			Options:     listOpts, Handler: listHandler(list),
		},
		{
			Name: "create_workflow", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: add.Scopes(), Operation: add.Root,
			Description: "Create an approval workflow in WSO2 Identity Server (who must approve, in which steps).",
			Options: []mcp.ToolOption{
				mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name.")),
				mcp.WithString("description", mcp.Description("Description.")),
				mcp.WithObject("template", mcp.Required(), mcp.Description("Approval template {name: MultiStepApprovalTemplate, steps: [{step, options: [{entity: roles|users, values: [...]}]}]}.")),
				mcp.WithString("engine", mcp.Description("Workflow engine; default "+defaultEngine+".")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "name")
				if err != nil {
					return nil, err
				}
				tpl, err := toolkit.OptObject(args, "template")
				if err != nil {
					return nil, err
				}
				body := map[string]any{"name": name, "engine": defaultEngine, "template": tpl}
				toolkit.PutIf(body, args, map[string]string{"description": "description", "engine": "engine"})
				return toolkit.Call(ctx, cl, add, nil, nil, body)
			},
		},
		{
			Name: "associate_workflow", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: associate.Scopes(), Operation: associate.Root,
			Description: "Attach an approval workflow to an operation in WSO2 Identity Server (for example ADD_USER or ADD_ROLE), so the operation waits for approval.",
			Options: []mcp.ToolOption{
				mcp.WithString("association_name", mcp.Required(), mcp.Description("Association name.")),
				mcp.WithString("operation", mcp.Required(), mcp.Description("Operation to guard, e.g. ADD_USER, DELETE_USER, ADD_ROLE, UPDATE_ROLES_OF_USERS.")),
				mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Workflow id from list_workflows.")),
				mcp.WithBoolean("is_enabled", mcp.Description("Enable the association; default true.")),
				mcp.WithObject("rule", mcp.Description("Optional rule that limits when the workflow applies.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{"isEnabled": true}
				for _, f := range [][2]string{{"association_name", "associationName"}, {"operation", "operation"}, {"workflow_id", "workflowId"}} {
					v, err := toolkit.RequireString(args, f[0])
					if err != nil {
						return nil, err
					}
					body[f[1]] = v
				}
				toolkit.PutIf(body, args, map[string]string{"is_enabled": "isEnabled", "rule": "rule"})
				return toolkit.Call(ctx, cl, associate, nil, nil, body)
			},
		},
		{
			Name: "list_workflow_requests", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: instances.Scopes(), Operation: instances.Root,
			Description: "List workflow requests (operations waiting for or done with approval) in WSO2 Identity Server.",
			Options:     listOpts, Handler: listHandler(instances),
		},
	}, nil
}
