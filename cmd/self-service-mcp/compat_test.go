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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/asgardeo/mcp/internal/selfservice"
	"github.com/mark3labs/mcp-go/mcp"
)

var updateGolden = flag.Bool("update", false, "update golden files")

type normalizedTool struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	InputSchema normalizedInputSchema `json:"inputSchema"`
	Annotations normalizedAnnotations `json:"annotations,omitempty"`
}

type normalizedInputSchema struct {
	Type       string            `json:"type,omitempty"`
	Required   []string          `json:"required,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type normalizedAnnotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

func TestEnduserToolsCompat(t *testing.T) {
	t.Setenv("BASE_URL", "https://localhost:9443")
	t.Setenv("CERT_PATH", "")
	t.Setenv("PORTAL_NAME", "")
	t.Setenv("CLIENT_ID", "")

	client, err := selfservice.NewClient("dummy-token")
	if err != nil {
		t.Fatalf("selfservice.NewClient: %v", err)
	}

	s := setupServer(client)
	respMsg := s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	rawResp, err := json.Marshal(respMsg)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}

	var rpc struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Type       string                    `json:"type"`
					Required   []string                  `json:"required"`
					Properties map[string]map[string]any `json:"properties"`
				} `json:"inputSchema"`
				Annotations mcp.ToolAnnotation `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(rawResp, &rpc); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	if rpc.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", rpc.Error)
	}
	if len(rpc.Result.Tools) != 28 {
		t.Fatalf("expected 28 enduser tools, got %d", len(rpc.Result.Tools))
	}

	got := make([]normalizedTool, 0, len(rpc.Result.Tools))
	for _, tl := range rpc.Result.Tools {
		req := append([]string(nil), tl.InputSchema.Required...)
		sort.Strings(req)

		var props map[string]string
		if len(tl.InputSchema.Properties) > 0 {
			props = make(map[string]string, len(tl.InputSchema.Properties))
			for k, v := range tl.InputSchema.Properties {
				typ, _ := v["type"].(string)
				props[k] = typ
			}
		}

		got = append(got, normalizedTool{
			Name:        tl.Name,
			Description: tl.Description,
			InputSchema: normalizedInputSchema{
				Type:       tl.InputSchema.Type,
				Required:   req,
				Properties: props,
			},
			Annotations: normalizedAnnotations{
				ReadOnlyHint:    tl.Annotations.ReadOnlyHint,
				DestructiveHint: tl.Annotations.DestructiveHint,
				IdempotentHint:  tl.Annotations.IdempotentHint,
				OpenWorldHint:   tl.Annotations.OpenWorldHint,
			},
		})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	goldenPath := filepath.Join("..", "..", "internal", "tools", "testdata", "enduser_tools.golden.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal golden: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	rawGolden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	var want []normalizedTool
	if err := json.Unmarshal(rawGolden, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("enduser tools mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}
