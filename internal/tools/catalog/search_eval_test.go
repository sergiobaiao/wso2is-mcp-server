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
	"os"
	"path/filepath"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
)

type searchEvalCase struct {
	Area       string   `json:"area"`
	Task       string   `json:"task"`
	ExpectedID string   `json:"expected_id"`
	Queries    []string `json:"queries"`
}

func TestSearchEval_Top10HitRate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "search_eval.json"))
	if err != nil {
		t.Fatalf("read search_eval.json: %v", err)
	}
	var cases []searchEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("unmarshal search_eval.json: %v", err)
	}
	if len(cases) < 30 {
		t.Fatalf("got %d eval cases, want at least 30", len(cases))
	}

	areas := make(map[string]bool)
	cat := load730Catalog(t)
	h := newTestHarness(t, cat, &config.Config{}, nil)

	hits := 0
	for i, tc := range cases {
		areas[tc.Area] = true
		if _, ok := cat.Get(tc.ExpectedID); !ok {
			t.Fatalf("eval case %d (%s): expected_id %q does not exist in 7.3.0 catalog", i, tc.Task, tc.ExpectedID)
		}
		if len(tc.Queries) == 0 || len(tc.Queries) > 2 {
			t.Fatalf("eval case %d (%s): expected 1 or 2 queries, got %d", i, tc.Task, len(tc.Queries))
		}

		matched := false
		for _, q := range tc.Queries {
			res, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
				"query": q,
				"limit": 10,
			})
			if res.IsError {
				t.Fatalf("search_operations(%q) failed: %s", q, text)
			}
			var out struct {
				Operations []struct {
					ID string `json:"id"`
				} `json:"operations"`
			}
			if err := json.Unmarshal([]byte(text), &out); err != nil {
				t.Fatalf("unmarshal search result: %v", err)
			}
			for _, op := range out.Operations {
				if op.ID == tc.ExpectedID {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			hits++
		} else {
			t.Logf("MISS case %d (%s): expected %s for queries %v", i, tc.Task, tc.ExpectedID, tc.Queries)
		}
	}

	if len(areas) < 10 {
		t.Errorf("covered %d areas, want at least 10", len(areas))
	}
	if hits < 27 {
		t.Errorf("search eval hit rate = %d/%d, want >= 27/%d", hits, len(cases), len(cases))
	}
}
