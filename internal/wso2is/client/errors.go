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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/redact"
)

// APIError represents a non-2xx HTTP response returned by WSO2 Identity Server.
type APIError struct {
	StatusCode int
	Body       []byte
	Message    string
	Scopes     []string
}

func newAPIError(statusCode int, rawBody []byte, op *catalog.Operation) *APIError {
	redacted := redact.JSONBytes(rawBody)
	var scopes []string
	if op != nil && len(op.Scopes) > 0 {
		scopes = append([]string(nil), op.Scopes...)
	}
	return &APIError{
		StatusCode: statusCode,
		Body:       redacted,
		Message:    extractISMessage(redacted),
		Scopes:     scopes,
	}
}

// Error implements error using the canonical IS error formatting.
func (e *APIError) Error() string {
	return FormatError(e, nil)
}

// FormatError formats an error per contracts/tool-conventions.md.
// For APIError, it renders status, hint, IS message, and required scopes on 403.
func FormatError(err error, op *catalog.Operation) string {
	if err == nil {
		return ""
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}

	hint := statusHint(apiErr.StatusCode)
	msg := strings.TrimSpace(apiErr.Message)

	var detail string
	switch {
	case hint != "" && msg != "":
		detail = hint + ". " + msg
	case hint != "":
		detail = hint
	case msg != "":
		detail = msg
	default:
		detail = http.StatusText(apiErr.StatusCode)
	}

	out := fmt.Sprintf("request failed with status %d: %s", apiErr.StatusCode, detail)

	if apiErr.StatusCode == http.StatusForbidden {
		scopes := apiErr.Scopes
		if len(scopes) == 0 && op != nil {
			scopes = op.Scopes
		}
		if len(scopes) > 0 {
			if !strings.HasSuffix(out, ".") {
				out += "."
			}
			out += fmt.Sprintf(" Required scopes: %s.", strings.Join(scopes, ", "))
		}
	}
	return out
}

// IsStatus reports whether err wraps an APIError with one of the given HTTP status codes.
func IsStatus(err error, statuses ...int) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return slices.Contains(statuses, apiErr.StatusCode)
}

func statusHint(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "the request was rejected by IS"
	case http.StatusUnauthorized:
		return "the access token is missing, expired or revoked"
	case http.StatusForbidden:
		return "the credentials lack permission"
	case http.StatusNotFound:
		return "not found, or not available on this server"
	case http.StatusConflict:
		return "it already exists or conflicts with the current state"
	default:
		return ""
	}
}

func extractISMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"detail", "description", "message", "error_description", "error"} {
		if v, ok := obj[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
