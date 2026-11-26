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

package workflows_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/tools/admin/admintest"
	"github.com/asgardeo/mcp/internal/tools/admin/workflows"
)

func TestWorkflowTools(t *testing.T) {
	e := admintest.New(t, workflows.Tools)
	e.Kinds(t, map[string][2]bool{
		"list_workflows": {true, false}, "create_workflow": {false, false},
		"associate_workflow": {false, false}, "list_workflow_requests": {true, false},
	})

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/workflows", http.StatusOK, map[string]any{"workflows": []any{}})
	e.MustCall(t, "list_workflows", map[string]any{"limit": 10})
	if q := e.Last(t).RawQuery; !strings.Contains(q, "limit=10") {
		t.Fatalf("list_workflows query = %q", q)
	}

	e.IS.HandleJSON(http.MethodPost, "/api/server/v1/workflows", http.StatusCreated, map[string]any{"id": "wf1"})
	e.MustCall(t, "create_workflow", map[string]any{
		"name": "approve-users", "template": map[string]any{"name": "MultiStepApprovalTemplate", "steps": []any{map[string]any{"step": 1, "options": []any{map[string]any{"entity": "roles", "values": []any{"r1"}}}}}},
	})
	if b := e.LastBody(t); b["name"] != "approve-users" || b["engine"] != "WorkflowEngine" {
		t.Fatalf("create_workflow body = %v", b)
	}

	e.IS.HandleJSON(http.MethodPost, "/api/server/v1/workflow-associations", http.StatusCreated, map[string]any{"id": "as1"})
	e.MustCall(t, "associate_workflow", map[string]any{"association_name": "on-add-user", "operation": "ADD_USER", "workflow_id": "wf1"})
	if b := e.LastBody(t); b["workflowId"] != "wf1" || b["operation"] != "ADD_USER" || b["isEnabled"] != true {
		t.Fatalf("associate_workflow body = %v", b)
	}

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/workflow-instances", http.StatusOK, map[string]any{"instances": []any{}})
	e.MustCall(t, "list_workflow_requests", map[string]any{"filter": "status eq PENDING"})
	e.Expect(t, http.MethodGet, "/api/server/v1/workflow-instances")
}
