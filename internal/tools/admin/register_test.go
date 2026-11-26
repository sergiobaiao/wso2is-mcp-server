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

package admin_test

import (
	"slices"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin"
	catalogtools "github.com/asgardeo/mcp/internal/tools/catalog"
	"github.com/asgardeo/mcp/internal/tools/scim"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
)

// defaultTools are the tool names of contracts/catalog-tools.md, contracts/scim-tools.md and
// contracts/admin-curated-tools.md: 4 catalog + 29 SCIM + 61 admin = 94 (research.md R13).
var defaultTools = []string{
	// catalog
	"search_operations", "describe_operation", "call_read_operation", "call_write_operation",
	// SCIM (21 ported + 8 gap tools)
	"list_users", "search_users", "get_user", "create_user", "update_user_attribute", "update_user_profile", "delete_user",
	"list_groups", "create_group", "add_user_to_group", "remove_user_from_group", "delete_group",
	"list_roles", "create_role", "assign_role_to_user", "remove_role_from_user", "delete_role",
	"list_agents", "create_agent", "delete_agent", "bulk_create_users",
	"patch_user", "get_group", "update_group", "get_role", "assign_role_to_group", "remove_role_from_group",
	"update_role_permissions", "get_agent",
	// applications (13 migrated + 2)
	"list_applications", "create_single_page_app", "create_webapp_with_ssr", "create_mobile_app", "create_m2m_app",
	"get_application_by_name", "get_application_by_client_id", "update_application_basic_info",
	"update_application_oauth_config", "update_application_claim_config", "authorize_api", "list_authorized_api",
	"update_login_flow", "delete_application", "share_application",
	// api_authorization (4 migrated + 5)
	"list_api_resources", "search_api_resources_by_name", "get_api_resource_by_identifier", "create_api_resource",
	"update_api_resource", "add_api_resource_scopes", "delete_api_resource", "list_oidc_scopes", "create_oidc_scope",
	// claims (1 migrated + 1)
	"list_claims", "create_local_claim",
	// identity_providers
	"list_identity_providers", "get_identity_provider", "create_identity_provider", "delete_identity_provider", "list_authenticators",
	// governance_security
	"list_governance_connectors", "get_governance_connector", "update_governance_connector", "update_validation_rules",
	// user_stores
	"list_user_stores", "get_user_store", "create_user_store", "test_user_store_connection",
	// user_lifecycle
	"list_user_sessions", "terminate_user_sessions",
	// organizations
	"list_organizations", "get_organization", "create_organization", "delete_organization", "share_users",
	// branding_notifications
	"get_branding", "update_branding", "configure_email_sender", "configure_sms_sender", "update_email_template",
	// extensions
	"list_webhooks", "create_webhook", "delete_webhook", "list_actions", "create_action", "delete_action",
	// workflows_approvals
	"list_workflows", "create_workflow", "associate_workflow", "list_workflow_requests",
}

func register(t *testing.T, cfg *config.Config) *toolkit.Registry {
	t.Helper()
	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	// T076: every catalog operation id a curated tool references exists, or Register fails.
	if err := catalogtools.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("catalog tools: %v", err)
	}
	if err := scim.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("SCIM tools: %v", err)
	}
	if err := admin.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("admin tools: %v", err)
	}
	return reg
}

// TestDefaultToolBudget covers T077 (SC-002): the default configuration registers at most 100
// tools, exactly the contract names.
func TestDefaultToolBudget(t *testing.T) {
	reg := register(t, &config.Config{ServerURL: "https://localhost:9443", TenantDomain: config.DefaultTenantDomain})
	got := reg.Names()
	want := slices.Clone(defaultTools)
	slices.Sort(got)
	slices.Sort(want)
	if len(got) > 100 {
		t.Fatalf("default registers %d tools, budget is 100", len(got))
	}
	if !slices.Equal(got, want) {
		for _, n := range want {
			if !slices.Contains(got, n) {
				t.Errorf("missing tool %s", n)
			}
		}
		for _, n := range got {
			if !slices.Contains(want, n) {
				t.Errorf("unexpected tool %s", n)
			}
		}
	}
}

// TestReadOnlyRegistersOnlyReadTools covers READ_ONLY=true for every layer.
func TestReadOnlyRegistersOnlyReadTools(t *testing.T) {
	reg := register(t, &config.Config{ServerURL: "https://localhost:9443", ReadOnly: true})
	for _, st := range reg.ServerTools() {
		if ro := st.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("READ_ONLY registered non-read tool %s", st.Tool.Name)
		}
	}
}
