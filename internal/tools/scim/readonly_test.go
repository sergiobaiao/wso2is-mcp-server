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

package scim_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
)

func TestReadOnlyRegistration(t *testing.T) {
	wantReadOnly := []string{
		"list_users",
		"search_users",
		"get_user",
		"list_groups",
		"get_group",
		"list_roles",
		"get_role",
		"list_agents",
		"get_agent",
	}

	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.ReadOnly = true
	})

	got := env.reg.Names()
	slices.Sort(got)
	wantSorted := append([]string(nil), wantReadOnly...)
	slices.Sort(wantSorted)

	if !slices.Equal(got, wantSorted) {
		t.Fatalf("READ_ONLY=true registered tools = %v, want %v", got, wantSorted)
	}

	for _, st := range env.reg.ServerTools() {
		ann := st.Tool.Annotations
		if ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
			t.Errorf("tool %s expected ReadOnlyHint=true", st.Tool.Name)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint {
			t.Errorf("tool %s expected DestructiveHint=false", st.Tool.Name)
		}
	}
}

func TestFullRegistrationCountAndConventions(t *testing.T) {
	env := setupTestEnv(t)
	if env.reg.Count() != 29 {
		t.Fatalf("expected 29 SCIM tools, got %d (%v)", env.reg.Count(), env.reg.Names())
	}

	const confirmSentence = "Before calling, describe the exact change and its target to the user and get their explicit yes to it."
	for _, st := range env.reg.ServerTools() {
		ann := st.Tool.Annotations
		isRead := ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
		if !isRead && !strings.Contains(st.Tool.Description, confirmSentence) {
			t.Errorf("write/destructive tool %s missing confirmation sentence in description", st.Tool.Name)
		}
	}
}
