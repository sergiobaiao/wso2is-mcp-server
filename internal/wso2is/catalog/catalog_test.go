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
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
)

var updateGolden = flag.Bool("update", false, "update catalog golden stats")

func TestLoad730Catalog(t *testing.T) {
	cat, err := Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("Load(7.3.0) failed: %v", err)
	}

	stats := cat.Stats()
	if stats.SpecFiles != 116 {
		t.Fatalf("SpecFiles = %d, want 116", stats.SpecFiles)
	}
	if stats.RootSpecFiles != 81 {
		t.Errorf("RootSpecFiles = %d, want 81", stats.RootSpecFiles)
	}
	if stats.OrgSpecFiles != 35 {
		t.Errorf("OrgSpecFiles = %d, want 35", stats.OrgSpecFiles)
	}

	// Verify KnownAreas in internal/config matches catalog.Areas().
	areas := cat.Areas()
	wantAreas := append([]string(nil), config.KnownAreas...)
	slices.Sort(wantAreas)
	if !slices.Equal(areas, wantAreas) {
		t.Errorf("catalog.Areas() = %v, config.KnownAreas = %v", areas, wantAreas)
	}

	// Verify uniqueness, prefixes, scopes, kinds.
	seenIDs := make(map[string]bool, len(cat.Operations))
	var piInfoOps int
	var flowUniqueOps int
	for _, op := range cat.Operations {
		if seenIDs[op.ID] {
			t.Fatalf("duplicate Operation.ID: %s", op.ID)
		}
		seenIDs[op.ID] = true

		if op.Family == FamilyRoot {
			if !strings.HasPrefix(op.ID, op.Area+".") || strings.HasPrefix(op.ID, op.Area+".org.") {
				t.Errorf("invalid root Operation.ID format: %s (area=%s)", op.ID, op.Area)
			}
		} else {
			if !strings.HasPrefix(op.ID, op.Area+".org.") {
				t.Errorf("invalid org Operation.ID format: %s (area=%s)", op.ID, op.Area)
			}
		}

		if op.Kind != KindRead && op.Kind != KindWrite && op.Kind != KindDestructive {
			t.Errorf("invalid kind %q on %s", op.Kind, op.ID)
		}
		if op.PathPrefix != "" && !strings.HasPrefix(op.Path, op.PathPrefix) {
			t.Errorf("operation %s path %q does not start with prefix %q", op.ID, op.Path, op.PathPrefix)
		}

		// Scopes must be sorted and unique.
		for i := 1; i < len(op.Scopes); i++ {
			if op.Scopes[i-1] >= op.Scopes[i] {
				t.Errorf("operation %s scopes not sorted/unique: %v", op.ID, op.Scopes)
				break
			}
		}

		if strings.Contains(op.Path, "/pi-info") {
			piInfoOps++
			if op.Flow {
				t.Errorf("pi-info operation %s should not be marked flow", op.ID)
			}
		}
		if op.Flow {
			flowUniqueOps++
		}
	}

	if piInfoOps != 2 {
		t.Errorf("expected 2 unique pi-info operations, got %d", piInfoOps)
	}
	if flowUniqueOps != 46 {
		t.Errorf("expected 46 unique flow operations, got %d", flowUniqueOps)
	}

	// Check Login Flow AI extras exist.
	for _, id := range []string{
		"applications.generateLoginFlow",
		"applications.getLoginFlowStatus",
		"applications.getLoginFlowResult",
	} {
		op, ok := cat.Get(id)
		if !ok {
			t.Fatalf("missing Login Flow AI extra %s", id)
		}
		if !op.AIFeature {
			t.Errorf("expected %s to have AIFeature=true", id)
		}
	}

	goldenPath := filepath.Join("testdata", "stats.golden.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			t.Fatalf("marshal stats: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	var want Stats
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if !reflect.DeepEqual(stats, want) {
		gotJSON, _ := json.MarshalIndent(stats, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("catalog stats mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}
