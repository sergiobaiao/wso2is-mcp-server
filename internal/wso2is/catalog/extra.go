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
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

// extraOperations returns operations used by frozen curated tools that are absent from the 7.3.0 OpenAPI specs (research.md R17).
func extraOperations() []*Operation {
	genBody := openapi3.NewObjectSchema().WithProperties(map[string]*openapi3.Schema{
		"user_query":               openapi3.NewStringSchema(),
		"available_authenticators": openapi3.NewArraySchema().WithItems(openapi3.NewStringSchema()),
	})
	idParam := Param{
		Name:     "id",
		In:       "path",
		Required: true,
		Schema:   openapi3.NewSchemaRef("", openapi3.NewStringSchema()),
	}

	return []*Operation{
		{
			ID:          "applications.generateLoginFlow",
			OperationID: "generateLoginFlow",
			Area:        "applications",
			Family:      FamilyRoot,
			Method:      http.MethodPost,
			Path:        "/api/server/v1/applications/loginflow/generate",
			PathPrefix:  "/api/server/v1",
			Summary:     "Start asynchronous Login Flow AI generation for an application.",
			Description: "Starts an asynchronous Login Flow AI generation job using the user prompt and available authenticators.",
			Kind:        KindWrite,
			Scopes:      []string{"internal_application_mgt_update"},
			Tags:        []string{"Applications", "Login Flow AI"},
			Body: &BodySpec{
				ContentType: "application/json",
				Required:    true,
				Schema:      openapi3.NewSchemaRef("", genBody),
			},
			AIFeature: true,
			Source:    "extra.go#POST /api/server/v1/applications/loginflow/generate",
		},
		{
			ID:          "applications.getLoginFlowStatus",
			OperationID: "getLoginFlowStatus",
			Area:        "applications",
			Family:      FamilyRoot,
			Method:      http.MethodGet,
			Path:        "/api/server/v1/applications/loginflow/status/{id}",
			PathPrefix:  "/api/server/v1",
			Summary:     "Get Login Flow AI generation status.",
			Description: "Polls the status of a Login Flow AI generation operation by operation id.",
			Kind:        KindRead,
			Scopes:      []string{"internal_application_mgt_view"},
			Tags:        []string{"Applications", "Login Flow AI"},
			Params:      []Param{idParam},
			AIFeature:   true,
			Source:      "extra.go#GET /api/server/v1/applications/loginflow/status/{id}",
		},
		{
			ID:          "applications.getLoginFlowResult",
			OperationID: "getLoginFlowResult",
			Area:        "applications",
			Family:      FamilyRoot,
			Method:      http.MethodGet,
			Path:        "/api/server/v1/applications/loginflow/result/{id}",
			PathPrefix:  "/api/server/v1",
			Summary:     "Get Login Flow AI generation result.",
			Description: "Fetches the generated authentication sequence from a completed Login Flow AI operation.",
			Kind:        KindRead,
			Scopes:      []string{"internal_application_mgt_view"},
			Tags:        []string{"Applications", "Login Flow AI"},
			Params:      []Param{idParam},
			AIFeature:   true,
			Source:      "extra.go#GET /api/server/v1/applications/loginflow/result/{id}",
		},
	}
}
