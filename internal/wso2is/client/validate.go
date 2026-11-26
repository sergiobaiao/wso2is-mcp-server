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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// ValidateInput validates pathParams, query, and body against op before any HTTP request is sent.
// Failures are formatted as "invalid input: <field>: <reason>; ...".
func ValidateInput(op *catalog.Operation, pathParams map[string]string, query map[string]any, body any) error {
	if op == nil {
		return errors.New("invalid input: operation: is required")
	}

	var issues []string

	// Build declared parameter lookup and OpenAPI operation.
	oaOp := openapi3.NewOperation()
	declaredQuery := make(map[string]catalog.Param)
	declaredPath := make(map[string]bool)

	for _, p := range op.Params {
		if p.In == openapi3.ParameterInHeader {
			continue
		}
		if p.In == openapi3.ParameterInQuery {
			declaredQuery[p.Name] = p
		}
		if p.In == openapi3.ParameterInPath {
			declaredPath[p.Name] = true
		}
		schema := p.Schema
		if schema == nil {
			schema = openapi3.NewSchemaRef("", openapi3.NewStringSchema())
		}
		param := &openapi3.Parameter{
			Name:     p.Name,
			In:       p.In,
			Required: p.Required || p.In == openapi3.ParameterInPath,
			Schema:   schema,
		}
		oaOp.AddParameter(param)
	}

	// Ensure every {placeholder} in op.Path is represented even if omitted from spec parameters.
	for _, match := range pathParamPattern.FindAllStringSubmatch(op.Path, -1) {
		name := match[1]
		if !declaredPath[name] {
			declaredPath[name] = true
			oaOp.AddParameter(openapi3.NewPathParameter(name).WithSchema(openapi3.NewStringSchema()))
		}
	}

	// Reject unknown query parameters and check scalar type compatibility before URL encoding.
	qValues := url.Values{}
	if len(query) > 0 {
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := query[k]
			p, ok := declaredQuery[k]
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: unknown query parameter", k))
				continue
			}
			if v == nil {
				continue
			}
			if typeErr := checkQueryParamType(k, p.Schema, v); typeErr != "" {
				issues = append(issues, typeErr)
				continue
			}
			addQueryValue(qValues, k, v)
		}
	}

	// Configure RequestBody on synthetic OpenAPI operation and prepare request body bytes.
	var bodyReader io.Reader
	var contentType string
	var contentLength int64

	if op.Body != nil {
		rb := openapi3.NewRequestBody().WithRequired(op.Body.Required)
		ct := op.Body.ContentType
		if ct == "" {
			ct = "application/json"
		}
		mt := openapi3.NewMediaType()
		if op.Body.Schema != nil {
			mt.Schema = op.Body.Schema
		}
		rb.Content = openapi3.Content{ct: mt}
		oaOp.RequestBody = &openapi3.RequestBodyRef{Value: rb}

		if body != nil {
			raw, encodedCT, err := encodeRequestBody(op.Body, body)
			if err != nil {
				issues = append(issues, formatEncodeError(err))
			} else {
				bodyReader = bytes.NewReader(raw)
				contentType = encodedCT
				contentLength = int64(len(raw))
			}
		}
	} else if body != nil {
		// Pass a non-empty body so RejectWhenRequestBodyNotSpecified flags it via openapi3filter.
		raw, err := encodeJSONBody(nil, body)
		if err != nil || len(raw) == 0 {
			raw = []byte("{}")
		}
		bodyReader = bytes.NewReader(raw)
		contentType = "application/json"
		contentLength = int64(len(raw))
	}

	// If encoding already failed for the body, return collected issues immediately
	// rather than masking them with a missing-body error.
	if len(issues) > 0 && op.Body != nil && body != nil && bodyReader == nil {
		return formatValidationIssues(issues)
	}

	rawURL := "https://is.local" + ensureLeadingSlash(op.Path)
	if len(qValues) > 0 {
		rawURL += "?" + qValues.Encode()
	}
	req, err := http.NewRequestWithContext(context.Background(), strings.ToUpper(op.Method), rawURL, bodyReader)
	if err != nil {
		return fmt.Errorf("invalid input: request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.ContentLength = contentLength

	pathItem := &openapi3.PathItem{}
	pathItem.SetOperation(strings.ToUpper(op.Method), oaOp)
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Paths:   openapi3.NewPaths(openapi3.WithPath(op.Path, pathItem)),
	}

	route := &routers.Route{
		Spec:      spec,
		Path:      op.Path,
		PathItem:  pathItem,
		Method:    strings.ToUpper(op.Method),
		Operation: oaOp,
	}

	input := &openapi3filter.RequestValidationInput{
		Request:     req,
		PathParams:  pathParams,
		QueryParams: qValues,
		Route:       route,
		Options: &openapi3filter.Options{
			MultiError:                        true,
			AuthenticationFunc:                openapi3filter.NoopAuthenticationFunc,
			RejectWhenRequestBodyNotSpecified: true,
			SkipSettingDefaults:               true,
		},
	}

	if err := openapi3filter.ValidateRequest(context.Background(), input); err != nil {
		issues = append(issues, extractFilterIssues(err)...)
	}

	if len(issues) > 0 {
		return formatValidationIssues(issues)
	}
	return nil
}

func checkQueryParamType(name string, schemaRef *openapi3.SchemaRef, v any) string {
	if schemaRef == nil || schemaRef.Value == nil || schemaRef.Value.Type == nil {
		return ""
	}
	t := schemaRef.Value.Type
	if t.Is(openapi3.TypeArray) {
		switch v.(type) {
		case []any, []string, []int, []float64, []bool:
			return ""
		default:
			return fmt.Sprintf("%s: expected array", name)
		}
	}
	switch v.(type) {
	case []any, []string, []int, []float64, []bool, map[string]any, map[string]string:
		return fmt.Sprintf("%s: expected scalar value", name)
	}

	switch {
	case t.Is(openapi3.TypeString):
		if _, ok := v.(string); !ok {
			return fmt.Sprintf("%s: expected string", name)
		}
	case t.Is(openapi3.TypeBoolean):
		switch v.(type) {
		case bool, string:
		default:
			return fmt.Sprintf("%s: expected boolean", name)
		}
	case t.Is(openapi3.TypeInteger):
		if _, ok := v.(bool); ok {
			return fmt.Sprintf("%s: expected integer", name)
		}
	case t.Is(openapi3.TypeNumber):
		if _, ok := v.(bool); ok {
			return fmt.Sprintf("%s: expected number", name)
		}
	}
	return ""
}

func addQueryValue(q url.Values, k string, v any) {
	switch items := v.(type) {
	case []string:
		for _, s := range items {
			q.Add(k, s)
		}
	case []any:
		for _, item := range items {
			q.Add(k, formatScalar(item))
		}
	default:
		q.Set(k, formatScalar(v))
	}
}

func formatEncodeError(err error) string {
	msg := err.Error()
	// Check for multipart field "<name>" error phrasing.
	const marker = `multipart field "`
	if idx := strings.Index(msg, marker); idx != -1 {
		rest := msg[idx+len(marker):]
		if end := strings.Index(rest, `"`); end != -1 {
			field := rest[:end]
			return fmt.Sprintf("%s: %s", field, msg)
		}
	}
	return fmt.Sprintf("body: %s", msg)
}

func extractFilterIssues(err error) []string {
	var out []string
	collectFilterIssues(err, &out)
	return out
}

func collectFilterIssues(err error, out *[]string) {
	if err == nil {
		return
	}

	var me openapi3.MultiError
	if errors.As(err, &me) {
		for _, sub := range me {
			collectFilterIssues(sub, out)
		}
		return
	}

	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.Parameter != nil {
			field := reqErr.Parameter.Name
			reason := extractInnerReason(reqErr.Err, reqErr.Reason)
			*out = append(*out, fmt.Sprintf("%s: %s", field, reason))
			return
		}
		if reqErr.RequestBody != nil {
			if reqErr.Err != nil {
				var subME openapi3.MultiError
				var se *openapi3.SchemaError
				switch {
				case errors.As(reqErr.Err, &subME):
					for _, sub := range subME {
						collectSchemaOrBodyIssue(sub, out)
					}
					return
				case errors.As(reqErr.Err, &se):
					collectSchemaOrBodyIssue(se, out)
					return
				}
			}
			reason := extractInnerReason(reqErr.Err, reqErr.Reason)
			*out = append(*out, fmt.Sprintf("body: %s", reason))
			return
		}
		reason := extractInnerReason(reqErr.Err, reqErr.Reason)
		*out = append(*out, fmt.Sprintf("body: %s", reason))
		return
	}

	collectSchemaOrBodyIssue(err, out)
}

func collectSchemaOrBodyIssue(err error, out *[]string) {
	var se *openapi3.SchemaError
	if errors.As(err, &se) {
		ptr := se.JSONPointer()
		field := strings.Join(ptr, ".")
		if field == "" {
			field = "body"
		}
		reason := se.Reason
		if reason == "" {
			reason = se.Error()
		}
		*out = append(*out, fmt.Sprintf("%s: %s", field, reason))
		return
	}
	*out = append(*out, fmt.Sprintf("body: %s", err.Error()))
}

func extractInnerReason(err error, fallback string) string {
	if err == nil {
		if fallback != "" {
			return fallback
		}
		return "invalid value"
	}
	var pe *openapi3filter.ParseError
	if errors.As(err, &pe) {
		if pe.Reason != "" {
			return pe.Reason
		}
		if pe.Cause != nil {
			return pe.Cause.Error()
		}
		return pe.Error()
	}
	var se *openapi3.SchemaError
	if errors.As(err, &se) && se.Reason != "" {
		return se.Reason
	}
	if fallback != "" {
		return fallback
	}
	return err.Error()
}

func formatValidationIssues(issues []string) error {
	seen := make(map[string]bool, len(issues))
	unique := make([]string, 0, len(issues))
	for _, issue := range issues {
		issue = strings.TrimSpace(issue)
		if issue == "" || seen[issue] {
			continue
		}
		seen[issue] = true
		unique = append(unique, issue)
	}
	return fmt.Errorf("invalid input: %s", strings.Join(unique, "; "))
}
