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

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// DefaultElicitationTimeout is the maximum duration to wait for user approval via elicitation.
	DefaultElicitationTimeout = 120 * time.Second
	// NotApprovedMessage is returned when an elicited write or destructive call is not approved.
	NotApprovedMessage = "Not approved by the user. Nothing was changed."
)

// ErrNotApproved indicates that an elicitation approval request was declined,
// cancelled, rejected, timed out, or failed.
var ErrNotApproved = errors.New(NotApprovedMessage)

// RequestApproval asks the connected MCP client to confirm a tool call via form
// elicitation. When the session or client does not support elicitation, it
// returns (true, false, nil) so the caller can proceed with the non-elicitation
// fallback. Any decline, cancellation, approve=false, timeout, or elicitation
// error returns (false, true, ErrNotApproved).
func RequestApproval(ctx context.Context, toolName, summary string, timeout time.Duration) (approved bool, supported bool, err error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return true, false, nil
	}

	elicitSession, ok := session.(server.SessionWithElicitation)
	if !ok {
		return true, false, nil
	}

	if infoSession, ok := session.(server.SessionWithClientInfo); ok {
		if infoSession.GetClientCapabilities().Elicitation == nil {
			return true, false, nil
		}
	}

	if timeout <= 0 {
		timeout = DefaultElicitationTimeout
	}

	elicitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := mcp.ElicitationRequest{
		Request: mcp.Request{
			Method: string(mcp.MethodElicitationCreate),
		},
		Params: mcp.ElicitationParams{
			Mode:    mcp.ElicitationModeForm,
			Message: fmt.Sprintf("%s: %s", toolName, summary),
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"approve": map[string]any{
						"type":  "boolean",
						"title": "Approve this change?",
					},
				},
				"required": []string{"approve"},
			},
		},
	}

	var res *mcp.ElicitationResult
	if srv := server.ServerFromContext(ctx); srv != nil {
		res, err = srv.RequestElicitation(elicitCtx, req)
	} else {
		res, err = elicitSession.RequestElicitation(elicitCtx, req)
	}
	if err != nil || res == nil {
		return false, true, ErrNotApproved
	}

	if res.Action != mcp.ElicitationResponseActionAccept {
		return false, true, ErrNotApproved
	}

	content, ok := res.Content.(map[string]any)
	if !ok {
		return false, true, ErrNotApproved
	}

	approve, ok := content["approve"].(bool)
	if !ok || !approve {
		return false, true, ErrNotApproved
	}

	return true, true, nil
}
