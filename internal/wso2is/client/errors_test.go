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

package client

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
)

func TestFormatErrorByStatus(t *testing.T) {
	op := &catalog.Operation{
		ID:     "users.getUser",
		Scopes: []string{"internal_user_mgt_view", "internal_user_mgt_list"},
	}

	cases := []struct {
		name   string
		err    *APIError
		op     *catalog.Operation
		wants  []string
		absent []string
	}{
		{
			name: "400 with IS description",
			err:  newAPIError(http.StatusBadRequest, []byte(`{"description":"Invalid filter syntax"}`), op),
			op:   op,
			wants: []string{
				"request failed with status 400: the request was rejected by IS. Invalid filter syntax",
			},
			absent: []string{"Required scopes"},
		},
		{
			name: "401 with SCIM detail",
			err:  newAPIError(http.StatusUnauthorized, []byte(`{"detail":"Token expired"}`), op),
			op:   op,
			wants: []string{
				"request failed with status 401: the access token is missing, expired or revoked. Token expired",
			},
		},
		{
			name: "403 includes required scopes from op",
			err:  newAPIError(http.StatusForbidden, []byte(`{"message":"Insufficient permissions"}`), op),
			op:   op,
			wants: []string{
				"request failed with status 403: the credentials lack permission.",
				"Insufficient permissions",
				"Required scopes: internal_user_mgt_view, internal_user_mgt_list.",
			},
		},
		{
			name: "403 with explicit APIError scopes and nil op",
			err: &APIError{
				StatusCode: http.StatusForbidden,
				Message:    "Access denied",
				Scopes:     []string{"internal_role_mgt_update"},
			},
			op: nil,
			wants: []string{
				"request failed with status 403: the credentials lack permission.",
				"Access denied",
				"Required scopes: internal_role_mgt_update.",
			},
		},
		{
			name: "404 empty body",
			err:  newAPIError(http.StatusNotFound, nil, op),
			op:   op,
			wants: []string{
				"request failed with status 404: not found, or not available on this server",
			},
		},
		{
			name: "409 conflict",
			err:  newAPIError(http.StatusConflict, []byte(`{"detail":"User already exists"}`), op),
			op:   op,
			wants: []string{
				"request failed with status 409: it already exists or conflicts with the current state. User already exists",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatError(tc.err, tc.op)
			for _, w := range tc.wants {
				if !strings.Contains(got, w) {
					t.Errorf("FormatError() = %q, want substring %q", got, w)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("FormatError() = %q, must not contain %q", got, a)
				}
			}
		})
	}
}

func TestAPIErrorRedactsSecrets(t *testing.T) {
	raw := []byte(`{"message":"Rejected secret","clientSecret":"top-secret-xyz","password":"pw-999"}`)
	apiErr := newAPIError(http.StatusBadRequest, raw, nil)

	if strings.Contains(string(apiErr.Body), "top-secret-xyz") || strings.Contains(string(apiErr.Body), "pw-999") {
		t.Fatalf("APIError.Body leaked secret: %s", string(apiErr.Body))
	}
	if !strings.Contains(string(apiErr.Body), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in APIError.Body: %s", string(apiErr.Body))
	}

	plain := FormatError(errors.New("plain failure"), nil)
	if plain != "plain failure" {
		t.Errorf("FormatError(non-APIError) = %q, want plain failure", plain)
	}
}
