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
	"fmt"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
)

// OpPair is the root and organization variant of the catalog operation a curated tool calls.
// Org is nil when the API has no organization variant.
type OpPair struct {
	Root *catalog.Operation
	Org  *catalog.Operation
}

// Lookup returns the catalog operation with id, or an error naming it. Curated tools resolve
// every operation at construction so a missing id fails startup and the tests.
func Lookup(cat *catalog.Catalog, id string) (*catalog.Operation, error) {
	if cat == nil {
		return nil, fmt.Errorf("catalog operation %q not found: no catalog", id)
	}
	op, ok := cat.Get(id)
	if !ok || op == nil {
		return nil, fmt.Errorf("catalog operation %q not found", id)
	}
	return op, nil
}

// LookupPair resolves the root operation rootID and, when orgID is not empty, its organization
// variant.
func LookupPair(cat *catalog.Catalog, rootID, orgID string) (OpPair, error) {
	root, err := Lookup(cat, rootID)
	if err != nil {
		return OpPair{}, err
	}
	p := OpPair{Root: root}
	if orgID != "" {
		if p.Org, err = Lookup(cat, orgID); err != nil {
			return OpPair{}, err
		}
	}
	return p, nil
}

// Select returns the operation for the call: Org when ctx targets a sub-organization (the
// toolkit puts organization_id on ctx), else Root.
func (p OpPair) Select(ctx context.Context) (*catalog.Operation, error) {
	cc, ok := client.CredentialContextFrom(ctx)
	if !ok || strings.TrimSpace(cc.OrganizationID) == "" {
		return p.Root, nil
	}
	if p.Org == nil {
		return nil, fmt.Errorf("operation %s is not available for organizations", p.Root.ID)
	}
	return p.Org, nil
}

// OrgCapable reports whether the pair has an organization variant.
func (p OpPair) OrgCapable() bool { return p.Org != nil }

// Scopes returns the sorted union of the scopes of both variants.
func (p OpPair) Scopes() []string {
	return UnionScopes(p.Root, p.Org)
}

// UnionScopes returns the sorted, unique scopes of ops (nil entries are skipped).
func UnionScopes(ops ...*catalog.Operation) []string {
	seen := map[string]bool{}
	var out []string
	for _, op := range ops {
		if op == nil {
			continue
		}
		for _, s := range op.Scopes {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// KeepDeclared returns the entries of q that op declares as query parameters. Organization
// variants often lack optional parameters of their root twin.
func KeepDeclared(op *catalog.Operation, q map[string]any) map[string]any {
	out := map[string]any{}
	for _, p := range op.Params {
		if v, ok := q[p.Name]; ok && p.In == "query" {
			out[p.Name] = v
		}
	}
	return out
}

// CallDeclared is Call with q reduced to the query parameters the selected operation declares.
func CallDeclared(ctx context.Context, cl *client.Client, p OpPair, path map[string]string, q map[string]any, body any) (any, error) {
	op, err := p.Select(ctx)
	if err != nil {
		return nil, err
	}
	return cl.Do(ctx, op, path, KeepDeclared(op, q), body)
}

// WithoutBodySchema returns a copy of op without its request body schema. Use it only for
// operations whose vendored schema rejects valid bodies (for example an ambiguous oneOf); the
// parameters are still validated and IS validates the body.
func WithoutBodySchema(op *catalog.Operation) *catalog.Operation {
	if op == nil || op.Body == nil || op.Body.Schema == nil {
		return op
	}
	copied := *op
	body := *op.Body
	body.Schema = nil
	copied.Body = &body
	return &copied
}

// CallUnvalidatedBody is Call without client-side body schema validation (see WithoutBodySchema).
func CallUnvalidatedBody(ctx context.Context, cl *client.Client, p OpPair, path map[string]string, q map[string]any, body any) (any, error) {
	op, err := p.Select(ctx)
	if err != nil {
		return nil, err
	}
	return cl.Do(ctx, WithoutBodySchema(op), path, q, body)
}

// Call selects the operation for ctx and sends it through cl.
func Call(ctx context.Context, cl *client.Client, p OpPair, path map[string]string, query map[string]any, body any) (any, error) {
	op, err := p.Select(ctx)
	if err != nil {
		return nil, err
	}
	return cl.Do(ctx, op, path, query, body)
}
