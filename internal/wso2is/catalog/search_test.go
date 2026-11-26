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
	"slices"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
)

func newTestCatalog(ops []*Operation) *Catalog {
	byID := make(map[string]*Operation, len(ops))
	for _, op := range ops {
		byID[op.ID] = op
	}
	sorted := append([]*Operation(nil), ops...)
	slices.SortFunc(sorted, func(a, b *Operation) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return &Catalog{
		Version:    "test",
		Operations: sorted,
		byID:       byID,
	}
}

func boolPtr(v bool) *bool { return &v }

func TestSearch_TokenizationAndFields(t *testing.T) {
	cat := newTestCatalog([]*Operation{
		{
			ID:          "governance_security.listAllowedCORSOrigins",
			OperationID: "listAllowedCORSOrigins",
			Area:        "governance_security",
			Family:      FamilyRoot,
			Method:      "GET",
			Path:        "/api/server/v1/cors/origins",
			Summary:     "Retrieve allowed CORS origins for the tenant.",
			Kind:        KindRead,
			Tags:        []string{"CORS", "Cross-Origin"},
		},
		{
			ID:          "users.getUserById",
			OperationID: "getUserById",
			Area:        "users",
			Family:      FamilyRoot,
			Method:      "GET",
			Path:        "/scim2/Users/{id}",
			Summary:     "Get a SCIM2 user resource.",
			Kind:        KindRead,
			Tags:        []string{"SCIM2", "Users"},
		},
		{
			ID:          "extensions.getWebhookMetadata",
			OperationID: "getWebhookMetadata",
			Area:        "extensions",
			Family:      FamilyRoot,
			Method:      "GET",
			Path:        "/api/server/v1/webhooks/metadata",
			Summary:     "Fetch event adapter metadata.",
			Kind:        KindRead,
			Tags:        []string{"webhook-metadata"},
		},
	})

	cases := []struct {
		name   string
		query  string
		wantID string
	}{
		{
			name:   "camelcase tokens in OperationID",
			query:  "allowed cors origins",
			wantID: "governance_security.listAllowedCORSOrigins",
		},
		{
			name:   "underscore separated area",
			query:  "governance_security",
			wantID: "governance_security.listAllowedCORSOrigins",
		},
		{
			name:   "slash separated path",
			query:  "/scim2/Users",
			wantID: "users.getUserById",
		},
		{
			name:   "dot separated canonical id",
			query:  "users.getUserById",
			wantID: "users.getUserById",
		},
		{
			name:   "hyphenated tag",
			query:  "webhook-metadata",
			wantID: "extensions.getWebhookMetadata",
		},
		{
			name:   "summary only token",
			query:  "adapter",
			wantID: "extensions.getWebhookMetadata",
		},
		{
			name:   "case insensitive uppercase",
			query:  "SCIM2",
			wantID: "users.getUserById",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches, total := cat.Search(tc.query, SearchFilter{})
			if total == 0 || len(matches) == 0 {
				t.Fatalf("Search(%q) returned no matches", tc.query)
			}
			if matches[0].ID != tc.wantID {
				t.Errorf("Search(%q) top match = %s, want %s", tc.query, matches[0].ID, tc.wantID)
			}
		})
	}
}

func TestSearch_Filters(t *testing.T) {
	var ops []*Operation
	for i := 0; i < 60; i++ {
		fam := FamilyRoot
		area := "users"
		kind := KindRead
		if i%2 == 1 {
			fam = FamilyOrg
		}
		if i%3 == 0 {
			area = "groups"
		}
		if i%5 == 0 {
			kind = KindWrite
		}
		id := fmt.Sprintf("%s.op%02d", area, i)
		if fam == FamilyOrg {
			id = fmt.Sprintf("%s.org.op%02d", area, i)
		}
		ops = append(ops, &Operation{
			ID:          id,
			OperationID: fmt.Sprintf("op%02d", i),
			Area:        area,
			Family:      fam,
			Method:      "GET",
			Path:        "/test/resource",
			Summary:     "Shared resource operation",
			Kind:        kind,
		})
	}
	cat := newTestCatalog(ops)

	t.Run("default limit 20", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{})
		if total != 60 {
			t.Errorf("total = %d, want 60", total)
		}
		if len(matches) != 20 {
			t.Errorf("len(matches) = %d, want 20", len(matches))
		}
	})

	t.Run("negative limit defaults to 20", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Limit: -5})
		if total != 60 || len(matches) != 20 {
			t.Errorf("got len=%d total=%d, want len=20 total=60", len(matches), total)
		}
	})

	t.Run("limit clamped to 50", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Limit: 100})
		if total != 60 {
			t.Errorf("total = %d, want 60", total)
		}
		if len(matches) != 50 {
			t.Errorf("len(matches) = %d, want 50", len(matches))
		}
	})

	t.Run("explicit limit within bounds", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Limit: 5})
		if total != 60 || len(matches) != 5 {
			t.Errorf("got len=%d total=%d, want len=5 total=60", len(matches), total)
		}
	})

	t.Run("area filter", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Area: "groups", Limit: 50})
		if total != 20 {
			t.Errorf("total = %d, want 20", total)
		}
		for _, op := range matches {
			if op.Area != "groups" {
				t.Errorf("got op %s in area %s, want groups", op.ID, op.Area)
			}
		}
	})

	t.Run("kind filter", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Kind: KindWrite, Limit: 50})
		if total != 12 {
			t.Errorf("total = %d, want 12", total)
		}
		for _, op := range matches {
			if op.Kind != KindWrite {
				t.Errorf("got op %s with kind %s, want write", op.ID, op.Kind)
			}
		}
	})

	t.Run("organization true", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Organization: boolPtr(true), Limit: 50})
		if total != 30 {
			t.Errorf("total = %d, want 30", total)
		}
		for _, op := range matches {
			if !op.OrgScoped() {
				t.Errorf("got root op %s when Organization=true", op.ID)
			}
		}
	})

	t.Run("organization false", func(t *testing.T) {
		matches, total := cat.Search("resource", SearchFilter{Organization: boolPtr(false), Limit: 50})
		if total != 30 {
			t.Errorf("total = %d, want 30", total)
		}
		for _, op := range matches {
			if op.OrgScoped() {
				t.Errorf("got org op %s when Organization=false", op.ID)
			}
		}
	})
}

func TestSearch_Ranking(t *testing.T) {
	t.Run("exact ID and OperationID outrank structural and summary matches", func(t *testing.T) {
		cat := newTestCatalog([]*Operation{
			{
				ID:          "applications.updateSomething",
				OperationID: "updateSomething",
				Area:        "applications",
				Family:      FamilyRoot,
				Path:        "/api/server/v1/applications",
				Summary:     "Mentions createRole in summary text",
				Kind:        KindWrite,
			},
			{
				ID:          "roles.createRole",
				OperationID: "createRole",
				Area:        "roles",
				Family:      FamilyRoot,
				Path:        "/scim2/v2/Roles",
				Summary:     "Create a role",
				Kind:        KindWrite,
			},
			{
				ID:          "roles.org.createRole",
				OperationID: "createRole",
				Area:        "roles",
				Family:      FamilyOrg,
				Path:        "/o/scim2/v2/Roles",
				Summary:     "Create a role in org",
				Kind:        KindWrite,
			},
			{
				ID:          "roles.createRoleFromTemplate",
				OperationID: "createRoleFromTemplate",
				Area:        "roles",
				Family:      FamilyRoot,
				Path:        "/scim2/v2/Roles/template",
				Summary:     "Create role from template",
				Kind:        KindWrite,
			},
		})

		// Exact ID match hits the org op specifically.
		matches, _ := cat.Search("roles.org.createRole", SearchFilter{})
		if len(matches) == 0 || matches[0].ID != "roles.org.createRole" {
			t.Fatalf("exact ID search top match = %v, want roles.org.createRole", matches)
		}

		// Exact OperationID match puts root non-deprecated before org.
		matches, _ = cat.Search("createRole", SearchFilter{})
		if len(matches) < 3 {
			t.Fatalf("expected at least 3 matches, got %d", len(matches))
		}
		if matches[0].ID != "roles.createRole" {
			t.Errorf("matches[0] = %s, want roles.createRole", matches[0].ID)
		}
		if matches[1].ID != "roles.org.createRole" {
			t.Errorf("matches[1] = %s, want roles.org.createRole", matches[1].ID)
		}
	})

	t.Run("all tokens in structural fields outrank summary and partial matches", func(t *testing.T) {
		cat := newTestCatalog([]*Operation{
			{
				ID:          "other.partialOp",
				OperationID: "partialOp",
				Area:        "other",
				Family:      FamilyRoot,
				Path:        "/api/server/v1/other",
				Summary:     "Only mentions session here",
				Kind:        KindRead,
			},
			{
				ID:          "other.summaryOnly",
				OperationID: "summaryOnly",
				Area:        "other",
				Family:      FamilyRoot,
				Path:        "/api/server/v1/misc",
				Summary:     "Terminate user session cleanly",
				Kind:        KindDestructive,
			},
			{
				ID:          "user_lifecycle.terminateUserSession",
				OperationID: "terminateUserSession",
				Area:        "user_lifecycle",
				Family:      FamilyRoot,
				Path:        "/api/server/v1/users/{userId}/sessions/{sessionId}",
				Summary:     "End an active session",
				Kind:        KindDestructive,
			},
		})

		matches, total := cat.Search("terminate user session", SearchFilter{})
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if matches[0].ID != "user_lifecycle.terminateUserSession" {
			t.Errorf("matches[0] = %s, want user_lifecycle.terminateUserSession", matches[0].ID)
		}
		if matches[1].ID != "other.summaryOnly" {
			t.Errorf("matches[1] = %s, want other.summaryOnly", matches[1].ID)
		}
		if matches[2].ID != "other.partialOp" {
			t.Errorf("matches[2] = %s, want other.partialOp", matches[2].ID)
		}
	})

	t.Run("tie-break non-deprecated root before deprecated and org, then op.ID", func(t *testing.T) {
		cat := newTestCatalog([]*Operation{
			{
				ID:          "roles.aDeprecated",
				OperationID: "aDeprecated",
				Area:        "roles",
				Family:      FamilyRoot,
				Path:        "/v1/roles",
				Summary:     "List items",
				Kind:        KindRead,
				Deprecated:  true,
			},
			{
				ID:          "roles.org.bOrg",
				OperationID: "bOrg",
				Area:        "roles",
				Family:      FamilyOrg,
				Path:        "/o/v2/roles",
				Summary:     "List items",
				Kind:        KindRead,
			},
			{
				ID:          "roles.dRoot",
				OperationID: "dRoot",
				Area:        "roles",
				Family:      FamilyRoot,
				Path:        "/v2/roles",
				Summary:     "List items",
				Kind:        KindRead,
			},
			{
				ID:          "roles.cRoot",
				OperationID: "cRoot",
				Area:        "roles",
				Family:      FamilyRoot,
				Path:        "/v2/roles",
				Summary:     "List items",
				Kind:        KindRead,
			},
		})

		matches, total := cat.Search("roles", SearchFilter{})
		if total != 4 {
			t.Fatalf("total = %d, want 4", total)
		}
		got := []string{matches[0].ID, matches[1].ID, matches[2].ID, matches[3].ID}
		want := []string{"roles.cRoot", "roles.dRoot", "roles.org.bOrg", "roles.aDeprecated"}
		if !slices.Equal(got, want) {
			t.Errorf("order = %v, want %v", got, want)
		}
	})
}

func TestSearch_EmptyQueryAndNilCatalog(t *testing.T) {
	var nilCat *Catalog
	if m, tot := nilCat.Search("users", SearchFilter{}); m != nil || tot != 0 {
		t.Errorf("nil catalog returned (%v, %d), want (nil, 0)", m, tot)
	}

	cat := newTestCatalog([]*Operation{
		{ID: "users.b", Area: "users", Family: FamilyRoot, Kind: KindRead},
		{ID: "users.a", Area: "users", Family: FamilyRoot, Kind: KindRead, Deprecated: true},
		{ID: "users.org.c", Area: "users", Family: FamilyOrg, Kind: KindRead},
		{ID: "groups.a", Area: "groups", Family: FamilyRoot, Kind: KindRead},
	})

	matches, total := cat.Search("   ", SearchFilter{Area: "users"})
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	got := []string{matches[0].ID, matches[1].ID, matches[2].ID}
	want := []string{"users.b", "users.org.c", "users.a"}
	if !slices.Equal(got, want) {
		t.Errorf("empty query order = %v, want %v", got, want)
	}
}

func TestSearch_730Catalog(t *testing.T) {
	cat, err := Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("Load(7.3.0) failed: %v", err)
	}

	matches, total := cat.Search("getCORSOrigins", SearchFilter{})
	if total == 0 || len(matches) == 0 {
		t.Fatal("expected match for getCORSOrigins")
	}
	if matches[0].ID != "governance_security.getCORSOrigins" {
		t.Errorf("top match = %s, want governance_security.getCORSOrigins", matches[0].ID)
	}

	matches2, total2 := cat.Search("cors origins", SearchFilter{})
	if total2 == 0 || matches2[0].ID != "governance_security.getCORSOrigins" {
		t.Errorf("phrase search top match = %v (total=%d), want governance_security.getCORSOrigins", matches2, total2)
	}

	orgMatches, orgTotal := cat.Search("application", SearchFilter{Organization: boolPtr(true), Limit: 50})
	if orgTotal == 0 {
		t.Fatal("expected org application matches")
	}
	for _, op := range orgMatches {
		if !op.OrgScoped() {
			t.Errorf("non-org op %s returned with Organization=true", op.ID)
		}
	}

	// Deterministic across repeated invocations.
	again, againTotal := cat.Search("application", SearchFilter{Organization: boolPtr(true), Limit: 50})
	if againTotal != orgTotal || len(again) != len(orgMatches) {
		t.Fatalf("non-deterministic total/len: (%d,%d) vs (%d,%d)", againTotal, len(again), orgTotal, len(orgMatches))
	}
	for i := range orgMatches {
		if orgMatches[i].ID != again[i].ID {
			t.Fatalf("non-deterministic order at %d: %s vs %s", i, orgMatches[i].ID, again[i].ID)
		}
	}
}
