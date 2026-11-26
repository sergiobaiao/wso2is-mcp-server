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
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// Kind describes the safety classification of an operation.
type Kind string

const (
	KindRead        Kind = "read"
	KindWrite       Kind = "write"
	KindDestructive Kind = "destructive"
)

// Param describes a normalized operation parameter.
type Param struct {
	Name     string              `json:"name"`
	In       string              `json:"in"`
	Required bool                `json:"required"`
	Schema   *openapi3.SchemaRef `json:"schema,omitempty"`
}

// BodySpec describes a normalized operation request body.
type BodySpec struct {
	ContentType string              `json:"content_type"`
	Schema      *openapi3.SchemaRef `json:"schema,omitempty"`
	Required    bool                `json:"required"`
}

// Operation represents one normalized IS HTTP operation in the catalog.
type Operation struct {
	ID          string    `json:"id"`
	OperationID string    `json:"operation_id"`
	Area        string    `json:"area"`
	Family      Family    `json:"family"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	PathPrefix  string    `json:"path_prefix,omitempty"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Kind        Kind      `json:"kind"`
	Scopes      []string  `json:"scopes,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Params      []Param   `json:"params,omitempty"`
	Body        *BodySpec `json:"body,omitempty"`
	Deprecated  bool      `json:"deprecated"`
	Flow        bool      `json:"flow"`
	AIFeature   bool      `json:"ai_feature,omitempty"`
	Source      string    `json:"source"`
}

// OrgScoped reports whether this operation targets sub-organizations (/o/).
func (o *Operation) OrgScoped() bool {
	return o.Family == FamilyOrg
}

var (
	scopeRegex   = regexp.MustCompile(`internal_[a-z0-9_]+`)
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	tenantSegRe  = regexp.MustCompile(`^.*?/t/\{[^}]+\}`)
)

var preferredBodyContentTypes = []string{
	"application/scim+json",
	"application/json",
	"application/x-www-form-urlencoded",
	"multipart/form-data",
}

var ignoredHeaders = map[string]bool{
	"authorization": true,
	"content-type":  true,
	"accept":        true,
}

var destructiveSegments = map[string]bool{
	"revoke":            true,
	"regenerate-secret": true,
	"terminate":         true,
	"unshare":           true,
	"remove":            true,
	"delete":            true,
	"disable":           true,
	"purge":             true,
}

func resolvePathPrefix(family Family, fileName string, servers []string, basePath string) string {
	if override, ok := prefixOverrides[string(family)+"/"+fileName]; ok {
		return override
	}
	if basePath != "" {
		return extractPrefixFromServerURL(basePath)
	}
	for _, s := range servers {
		if strings.Contains(s, "swaggerhub.com") || strings.Contains(s, "asgardeo.io") {
			continue
		}
		if p := extractPrefixFromServerURL(s); p != "" {
			return p
		}
	}
	if len(servers) > 0 {
		return extractPrefixFromServerURL(servers[0])
	}
	return ""
}

func extractPrefixFromServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Strip scheme + host placeholder if present (including malformed https:/{serverUrl}).
	pathPart := raw
	if idx := strings.Index(raw, "://"); idx != -1 {
		rest := raw[idx+3:]
		if slash := strings.IndexByte(rest, '/'); slash != -1 {
			pathPart = rest[slash:]
		} else {
			pathPart = ""
		}
	} else if strings.HasPrefix(raw, "https:/") || strings.HasPrefix(raw, "http:/") {
		if slash := strings.Index(raw, ":/"); slash != -1 {
			rest := strings.TrimPrefix(raw[slash+2:], "/")
			if next := strings.IndexByte(rest, '/'); next != -1 {
				pathPart = rest[next:]
			} else {
				pathPart = ""
			}
		}
	}

	if tenantSegRe.MatchString(pathPart) {
		pathPart = tenantSegRe.ReplaceAllString(pathPart, "")
	}
	pathPart = strings.TrimRight(pathPart, "/")
	if pathPart != "" && !strings.HasPrefix(pathPart, "/") {
		pathPart = "/" + pathPart
	}
	return pathPart
}

func joinPath(prefix, relPath string) string {
	if prefix == "" {
		if strings.HasPrefix(relPath, "/") {
			return relPath
		}
		return "/" + relPath
	}
	if relPath == "" || relPath == "/" {
		return prefix
	}
	if !strings.HasPrefix(relPath, "/") {
		relPath = "/" + relPath
	}
	if strings.HasPrefix(relPath, prefix+"/") || relPath == prefix {
		return relPath
	}
	return prefix + relPath
}

func deriveOperationID(method, rawPath string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))

	segments := strings.Split(strings.Trim(rawPath, "/"), "/")
	wroteSegment := false
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		wroteSegment = true
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			paramName := strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "}")
			b.WriteString("By")
			b.WriteString(toPascalCase(paramName))
			continue
		}
		b.WriteString(toPascalCase(seg))
	}
	if !wroteSegment {
		b.WriteString("Root")
	}
	return b.String()
}

func sanitizeOperationID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// If already valid lowerCamelCase ASCII (no spaces, underscores, hyphens, or symbols), keep casing.
	if isCleanCamel(raw) {
		runes := []rune(raw)
		runes[0] = unicode.ToLower(runes[0])
		return string(runes)
	}

	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(parts) == 0 {
		return ""
	}

	var out strings.Builder
	for i, p := range parts {
		runes := []rune(p)
		if i == 0 {
			out.WriteString(strings.ToLower(string(runes[0])))
			out.WriteString(string(runes[1:]))
		} else {
			out.WriteString(strings.ToUpper(string(runes[0])))
			out.WriteString(string(runes[1:]))
		}
	}
	return out.String()
}

func isCleanCamel(s string) bool {
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func toPascalCase(s string) string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()

	var out strings.Builder
	for _, p := range parts {
		runes := []rune(p)
		out.WriteString(strings.ToUpper(string(runes[0])))
		out.WriteString(strings.ToLower(string(runes[1:])))
	}
	return out.String()
}

func classifyKind(method, fullPath string) Kind {
	m := strings.ToUpper(method)
	switch m {
	case http.MethodGet, http.MethodHead:
		return KindRead
	case http.MethodDelete:
		return KindDestructive
	}

	if m == http.MethodPost && strings.HasSuffix(fullPath, "/.search") {
		return KindRead
	}
	if m == http.MethodPut && (strings.HasSuffix(fullPath, "/scim2/Groups/{id}") || strings.HasSuffix(fullPath, "/scim2/v2/Roles/{id}")) {
		return KindDestructive
	}

	for _, seg := range strings.Split(strings.Trim(fullPath, "/"), "/") {
		if destructiveSegments[strings.ToLower(seg)] {
			return KindDestructive
		}
	}
	return KindWrite
}

func extractScopes(op *openapi3.Operation) []string {
	set := make(map[string]bool)
	for _, m := range scopeRegex.FindAllString(op.Description, -1) {
		set[m] = true
	}
	if op.Security != nil {
		for _, req := range *op.Security {
			for _, scopes := range req {
				for _, s := range scopes {
					if scopeRegex.MatchString(s) {
						set[s] = true
					}
				}
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

func cleanText(raw string) string {
	stripped := htmlTagRegex.ReplaceAllString(raw, " ")
	fields := strings.Fields(stripped)
	return strings.Join(fields, " ")
}

func buildSummary(summary, cleanedDesc string) string {
	s := cleanText(summary)
	if s == "" {
		s = cleanedDesc
		if idx := strings.Index(s, ". "); idx != -1 {
			s = s[:idx+1]
		}
	}
	if len(s) > 200 {
		s = strings.TrimSpace(s[:197]) + "..."
	}
	return s
}

func extractParams(pathParams, opParams openapi3.Parameters) []Param {
	byKey := make(map[string]Param)
	order := make([]string, 0, len(pathParams)+len(opParams))

	add := func(ref *openapi3.ParameterRef) {
		if ref == nil || ref.Value == nil {
			return
		}
		p := ref.Value
		in := strings.ToLower(p.In)
		if in == "cookie" {
			return
		}
		if in == "header" && ignoredHeaders[strings.ToLower(p.Name)] {
			return
		}
		key := in + ":" + p.Name
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = Param{
			Name:     p.Name,
			In:       in,
			Required: p.Required || in == "path",
			Schema:   p.Schema,
		}
	}

	for _, p := range pathParams {
		add(p)
	}
	for _, p := range opParams {
		add(p)
	}

	out := make([]Param, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}

func extractBody(rb *openapi3.RequestBodyRef) *BodySpec {
	if rb == nil || rb.Value == nil || len(rb.Value.Content) == 0 {
		return nil
	}
	for _, ct := range preferredBodyContentTypes {
		if mt, ok := rb.Value.Content[ct]; ok && mt != nil {
			return &BodySpec{
				ContentType: ct,
				Schema:      mt.Schema,
				Required:    rb.Value.Required,
			}
		}
	}
	// Fallback to first content type in sorted order if none of the preferred ones match.
	cts := make([]string, 0, len(rb.Value.Content))
	for ct := range rb.Value.Content {
		cts = append(cts, ct)
	}
	sort.Strings(cts)
	ct := cts[0]
	return &BodySpec{
		ContentType: ct,
		Schema:      rb.Value.Content[ct].Schema,
		Required:    rb.Value.Required,
	}
}

func isFlowOperation(sf *SpecFile, method, fullPath string) bool {
	if strings.HasSuffix(fullPath, "/pi-info") || strings.HasSuffix(fullPath, "/pi-info/{userId}") {
		return false
	}
	if sf.Flow {
		return true
	}
	m := strings.ToUpper(method)
	if m == http.MethodPost && strings.HasSuffix(fullPath, "/me/change-password") {
		return true
	}
	if m == http.MethodDelete && strings.HasSuffix(fullPath, "/scim2/Me") {
		return true
	}
	return false
}

func makeOperationID(area string, fam Family, opID string) string {
	if fam == FamilyOrg {
		return fmt.Sprintf("%s.org.%s", area, opID)
	}
	return fmt.Sprintf("%s.%s", area, opID)
}
