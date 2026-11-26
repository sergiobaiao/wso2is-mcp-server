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

package client

import (
	"context"

	"github.com/asgardeo/mcp/internal/config"
)

// CredentialContext carries per-call authentication and organization context.
type CredentialContext struct {
	Mode           config.AuthMode
	BearerToken    string
	Subject        string
	OrganizationID string
}

type credContextKey struct{}

// WithCredentialContext attaches cc to ctx.
func WithCredentialContext(ctx context.Context, cc CredentialContext) context.Context {
	return context.WithValue(ctx, credContextKey{}, cc)
}

// CredentialContextFrom retrieves the CredentialContext attached to ctx, if any.
func CredentialContextFrom(ctx context.Context) (CredentialContext, bool) {
	if ctx == nil {
		return CredentialContext{}, false
	}
	cc, ok := ctx.Value(credContextKey{}).(CredentialContext)
	return cc, ok
}

// WithOrganizationID returns a derived context whose CredentialContext targets orgID.
func WithOrganizationID(ctx context.Context, orgID string) context.Context {
	cc, _ := CredentialContextFrom(ctx)
	cc.OrganizationID = orgID
	return WithCredentialContext(ctx, cc)
}
