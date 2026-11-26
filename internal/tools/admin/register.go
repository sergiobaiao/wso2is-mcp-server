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

// Package admin registers the curated administration tools of every area
// (contracts/admin-curated-tools.md).
package admin

import (
	"fmt"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/apiauth"
	"github.com/asgardeo/mcp/internal/tools/admin/claims"
	"github.com/asgardeo/mcp/internal/tools/admin/governance"
	"github.com/asgardeo/mcp/internal/tools/admin/idp"
	"github.com/asgardeo/mcp/internal/tools/admin/sessions"
	"github.com/asgardeo/mcp/internal/tools/admin/userstores"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
)

type areaTools func(*catalog.Catalog, *client.Client, *config.Config) ([]toolkit.ToolDef, error)

// areas lists the migrated and new admin areas in registration order. Areas still to come are
// tracked in specs/001-wso2is-full-coverage/tasks.md (US5).
var areas = []struct {
	name  string
	tools areaTools
}{
	{"api_authorization", apiauth.Tools},
	{"claims", claims.Tools},
	{"identity_providers", idp.Tools},
	{"governance_security", governance.Tools},
	{"user_stores", userstores.Tools},
	{"user_lifecycle", sessions.Tools},
}

// Register adds the curated admin tools of every area to reg. A missing catalog operation is an
// error, so a broken id fails startup.
func Register(reg *toolkit.Registry, cat *catalog.Catalog, cl *client.Client, cfg *config.Config) error {
	for _, a := range areas {
		defs, err := a.tools(cat, cl, cfg)
		if err != nil {
			return fmt.Errorf("admin area %s: %w", a.name, err)
		}
		for _, def := range defs {
			if err := reg.Register(def); err != nil {
				return fmt.Errorf("admin area %s: %w", a.name, err)
			}
		}
	}
	return nil
}
