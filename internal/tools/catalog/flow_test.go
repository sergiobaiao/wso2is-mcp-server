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

package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	iscatalog "github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestFlowAPIs_ClassificationAndGating(t *testing.T) {
	cat := load730Catalog(t)

	var flowOps []*iscatalog.Operation
	var piInfoOps []*iscatalog.Operation
	for _, op := range cat.Operations {
		if op.Flow {
			flowOps = append(flowOps, op)
		}
		if strings.Contains(op.Path, "/pi-info") {
			piInfoOps = append(piInfoOps, op)
		}
	}

	if len(flowOps) != 46 {
		t.Fatalf("unique flow operations = %d, want 46", len(flowOps))
	}
	if len(piInfoOps) != 2 {
		t.Fatalf("unique pi-info operations = %d, want 2", len(piInfoOps))
	}
	for _, op := range piInfoOps {
		if op.Flow {
			t.Errorf("pi-info operation %s must NOT be classified as flow", op.ID)
		}
	}

	t.Run("AllowFlowAPIs=false disables all flow ops and keeps pi-info available", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{
			AllowFlowAPIs:     false,
			IncludeDeprecated: true,
		}, nil)
		h.srv.SetFallback(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		})

		// Verify all 46 flow ops are marked flow_disabled in describe and refused in call_* with zero HTTP calls.
		for _, op := range flowOps {
			res, descText := callTool(t, h.mcpClient, "describe_operation", map[string]any{
				"operation_id": op.ID,
			})
			if res.IsError {
				t.Fatalf("describe_operation(%s) failed: %s", op.ID, descText)
			}
			var desc map[string]any
			if err := json.Unmarshal([]byte(descText), &desc); err != nil {
				t.Fatalf("unmarshal describe(%s): %v", op.ID, err)
			}
			if desc["availability"] != "flow_disabled" {
				t.Errorf("%s availability = %v, want flow_disabled", op.ID, desc["availability"])
			}
			reason, _ := desc["availability_reason"].(string)
			if !strings.Contains(reason, "ALLOW_FLOW_APIS=true") {
				t.Errorf("%s availability_reason = %q, want ALLOW_FLOW_APIS=true", op.ID, reason)
			}

			toolName := "call_write_operation"
			if op.Kind == iscatalog.KindRead {
				toolName = "call_read_operation"
			}
			args := buildMinimalCallArgs(op)
			h.srv.ResetRequests()
			callRes, callText := callTool(t, h.mcpClient, toolName, args)
			if !callRes.IsError {
				t.Fatalf("%s(%s) expected error when AllowFlowAPIs=false, got: %s", toolName, op.ID, callText)
			}
			if !strings.Contains(callText, "ALLOW_FLOW_APIS=true") {
				t.Errorf("%s(%s) error = %q, want ALLOW_FLOW_APIS=true", toolName, op.ID, callText)
			}
			if reqs := h.srv.Requests(); len(reqs) != 0 {
				t.Fatalf("%s(%s) sent %d HTTP requests when flow disabled", toolName, op.ID, len(reqs))
			}
		}

		// Verify pi-info operations are NOT flow-disabled.
		for _, op := range piInfoOps {
			res, descText := callTool(t, h.mcpClient, "describe_operation", map[string]any{
				"operation_id": op.ID,
			})
			if res.IsError {
				t.Fatalf("describe_operation(%s) failed: %s", op.ID, descText)
			}
			var desc map[string]any
			if err := json.Unmarshal([]byte(descText), &desc); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if desc["availability"] != "available" {
				t.Errorf("pi-info %s availability = %v, want available", op.ID, desc["availability"])
			}
		}
	})

	t.Run("AllowFlowAPIs=true enables flow ops through call_read_operation and call_write_operation with elicitation", func(t *testing.T) {
		var elicited atomic.Bool
		h := newTestHarness(t, cat, &config.Config{
			AllowFlowAPIs:     true,
			IncludeDeprecated: true,
		}, func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			elicited.Store(true)
			return &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{
					Action:  mcp.ElicitationResponseActionAccept,
					Content: map[string]any{"approve": true},
				},
			}, nil
		})
		h.srv.SetFallback(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		})

		// Test a write flow operation (e.g. user_lifecycle.changePassword).
		elicited.Store(false)
		op, ok := cat.Get("user_lifecycle.changePassword")
		if !ok {
			t.Fatal("missing user_lifecycle.changePassword")
		}
		res, text := callTool(t, h.mcpClient, "call_write_operation", buildMinimalCallArgs(op))
		if res.IsError {
			t.Fatalf("call_write_operation(changePassword) failed: %s", text)
		}
		if !elicited.Load() {
			t.Fatal("expected elicitation for flow write operation")
		}

		// Test a read flow operation (find first read flow op).
		var readFlowOp *iscatalog.Operation
		for _, fop := range flowOps {
			if fop.Kind == iscatalog.KindRead {
				readFlowOp = fop
				break
			}
		}
		if readFlowOp == nil {
			t.Fatal("expected at least one read flow operation in 7.3.0")
		}
		resR, textR := callTool(t, h.mcpClient, "call_read_operation", buildMinimalCallArgs(readFlowOp))
		if resR.IsError {
			t.Fatalf("call_read_operation(%s) failed: %s", readFlowOp.ID, textR)
		}
	})

	t.Run("AllowFlowAPIs=true respects elicitation rejection", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{
			AllowFlowAPIs: true,
		}, func(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{
					Action: mcp.ElicitationResponseActionDecline,
				},
			}, nil
		})
		op, ok := cat.Get("user_lifecycle.changePassword")
		if !ok {
			t.Fatal("missing user_lifecycle.changePassword")
		}
		h.srv.ResetRequests()
		res, text := callTool(t, h.mcpClient, "call_write_operation", buildMinimalCallArgs(op))
		if !res.IsError || text != toolkit.NotApprovedMessage {
			t.Fatalf("got (isError=%v, text=%q), want NotApprovedMessage", res.IsError, text)
		}
		if len(h.srv.Requests()) != 0 {
			t.Fatal("expected zero HTTP requests when elicitation declined")
		}
	})
}
