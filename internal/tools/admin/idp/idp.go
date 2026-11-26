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

// Package idp implements the curated identity_providers tools (contracts/admin-curated-tools.md).
package idp

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// template maps tool inputs onto one IS federated authenticator. inputs lists
// {tool input, authenticator property key, required}; fixed holds constant properties.
type template struct {
	authenticator string
	inputs        [][3]string
	fixed         map[string]string
}

const req, opt = "required", ""

var oauthClient = [][3]string{{"client_id", "ClientId", req}, {"client_secret", "ClientSecret", req}}

// templates are the identity provider templates of the contract. Property keys are the IS
// authenticator property names.
var templates = map[string]template{
	"google":   {authenticator: "GoogleOIDCAuthenticator", inputs: oauthClient},
	"github":   {authenticator: "GithubAuthenticator", inputs: oauthClient},
	"facebook": {authenticator: "FacebookAuthenticator", inputs: oauthClient},
	"microsoft": {
		authenticator: "OpenIDConnectAuthenticator", inputs: oauthClient,
		fixed: map[string]string{
			"OAuth2AuthzEPUrl": "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			"OAuth2TokenEPUrl": "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			"Scopes":           "openid email profile",
		},
	},
	"apple": {authenticator: "AppleOIDCAuthenticator", inputs: [][3]string{
		{"client_id", "ClientId", req}, {"team_id", "TeamId", req}, {"key_id", "KeyId", req},
		{"private_key", "PrivateKey", req},
	}},
	"oidc": {authenticator: "OpenIDConnectAuthenticator", inputs: [][3]string{
		{"client_id", "ClientId", req}, {"client_secret", "ClientSecret", req},
		{"authorization_endpoint", "OAuth2AuthzEPUrl", req}, {"token_endpoint", "OAuth2TokenEPUrl", req},
		{"userinfo_endpoint", "UserInfoUrl", opt}, {"logout_endpoint", "OIDCLogoutEPUrl", opt},
		{"scopes", "Scopes", opt},
	}},
	"saml": {authenticator: "SAMLSSOAuthenticator", inputs: [][3]string{
		{"idp_entity_id", "IdPEntityId", req}, {"sso_url", "SSOUrl", req},
		{"sp_entity_id", "SPEntityId", opt}, {"logout_url", "LogoutReqUrl", opt},
	}},
}

func templateNames() []string {
	names := make([]string, 0, len(templates))
	for n := range templates {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Tools returns the identity provider tools.
func Tools(cat *catalog.Catalog, cl *client.Client, cfg *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"identity_providers.getIDPs", "identity_providers.org.getIDPs"},
		{"identity_providers.getIDP", "identity_providers.org.getIDP"},
		{"identity_providers.addIDP", "identity_providers.org.addIDP"},
		{"identity_providers.deleteIDP", "identity_providers.org.deleteIDP"},
		{"identity_providers.getAuthenticators", "identity_providers.org.getAuthenticators"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	list, get, add, del, authenticators := ops[0], ops[1], ops[2], ops[3], ops[4]
	if cfg == nil {
		cfg = &config.Config{}
	}

	idpID := mcp.WithString("identity_provider_id", mcp.Required(), mcp.Description("Identity provider id, as returned by list_identity_providers."))
	listQuery := func(args map[string]any) (map[string]any, error) {
		q, err := toolkit.Paging(args)
		if err != nil {
			return nil, err
		}
		filter, err := toolkit.OptString(args, "filter")
		if err != nil {
			return nil, err
		}
		if filter != "" {
			q["filter"] = filter
		}
		return q, nil
	}
	pagingOpts := []mcp.ToolOption{
		mcp.WithNumber("limit", mcp.Description("Maximum number of results.")),
		mcp.WithNumber("offset", mcp.Description("Number of results to skip.")),
		mcp.WithString("filter", mcp.Description("Filter expression, for example: name co Google.")),
	}

	createOpts := []mcp.ToolOption{
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the identity provider.")),
		mcp.WithString("template", mcp.Required(), mcp.Enum(templateNames()...),
			mcp.Description("Identity provider template. google, github, facebook and microsoft need client_id and client_secret; apple needs client_id, team_id, key_id and private_key; oidc needs client_id, client_secret, authorization_endpoint and token_endpoint; saml needs idp_entity_id and sso_url.")),
		mcp.WithString("description", mcp.Description("Description of the identity provider.")),
		mcp.WithString("callback_url", mcp.Description("Redirect URL registered at the external provider. Defaults to the IS commonauth endpoint.")),
	}
	seen := map[string]bool{}
	for _, name := range templateNames() {
		for _, in := range templates[name].inputs {
			if !seen[in[0]] {
				seen[in[0]] = true
				createOpts = append(createOpts, mcp.WithString(in[0], mcp.Description("Template input "+in[0]+" (IS property "+in[1]+"). Secrets are sent to IS and never returned.")))
			}
		}
	}

	return []toolkit.ToolDef{
		{
			Name: "list_identity_providers", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: list.Scopes(), OrgCapable: true, Operation: list.Root,
			Description: "List the identity providers (federated connections such as Google or a corporate SAML IdP) configured in WSO2 Identity Server.",
			Options:     pagingOpts,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				q, err := listQuery(args)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, list, nil, q, nil)
			},
		},
		{
			Name: "get_identity_provider", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: get.Scopes(), OrgCapable: true, Operation: get.Root,
			Description: "Get one identity provider of WSO2 Identity Server with its authenticators (secrets are redacted).",
			Options:     []mcp.ToolOption{idpID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "identity_provider_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, get, map[string]string{"identity-provider-id": id}, nil, nil)
			},
		},
		{
			Name: "create_identity_provider", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: add.Scopes(), OrgCapable: true, Operation: add.Root,
			Description: "Create an identity provider in WSO2 Identity Server from a template (google, github, facebook, microsoft, apple, oidc, saml).",
			Options:     createOpts,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body, err := buildIDP(args, cfg)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, add, nil, nil, body)
			},
		},
		{
			Name: "delete_identity_provider", Layer: toolkit.LayerCurated, Kind: catalog.KindDestructive,
			Scopes: del.Scopes(), OrgCapable: true, Operation: del.Root,
			Description: "Delete an identity provider from WSO2 Identity Server. Applications that use it for login lose that option.",
			Options:     []mcp.ToolOption{idpID},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "identity_provider_id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, del, map[string]string{"identity-provider-id": id}, nil, nil)
			},
		},
		{
			Name: "list_authenticators", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: authenticators.Scopes(), OrgCapable: true, Operation: authenticators.Root,
			Description: "List the local and federated authenticators available in WSO2 Identity Server for login flows.",
			Options:     pagingOpts,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				q, err := listQuery(args)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, authenticators, nil, q, nil)
			},
		},
	}, nil
}

// buildIDP builds the addIDP request body for the chosen template.
func buildIDP(args map[string]any, cfg *config.Config) (map[string]any, error) {
	name, err := toolkit.RequireString(args, "name")
	if err != nil {
		return nil, err
	}
	tname, err := toolkit.RequireString(args, "template")
	if err != nil {
		return nil, err
	}
	tpl, ok := templates[tname]
	if !ok {
		return nil, fmt.Errorf("invalid input: template: must be one of %s", strings.Join(templateNames(), ", "))
	}

	var props []any
	for _, in := range tpl.inputs {
		v, err := toolkit.OptString(args, in[0])
		if err != nil {
			return nil, err
		}
		if v == "" {
			if in[2] == req {
				return nil, fmt.Errorf("invalid input: %s: is required for template %s", in[0], tname)
			}
			continue
		}
		props = append(props, map[string]any{"key": in[1], "value": v})
	}
	fixedKeys := make([]string, 0, len(tpl.fixed))
	for k := range tpl.fixed {
		fixedKeys = append(fixedKeys, k)
	}
	sort.Strings(fixedKeys)
	for _, k := range fixedKeys {
		props = append(props, map[string]any{"key": k, "value": tpl.fixed[k]})
	}
	if tname != "saml" {
		callback, err := toolkit.OptString(args, "callback_url")
		if err != nil {
			return nil, err
		}
		if callback == "" {
			callback = commonAuthURL(cfg)
		}
		props = append(props, map[string]any{"key": "callbackUrl", "value": callback})
	}

	authID := base64.RawURLEncoding.EncodeToString([]byte(tpl.authenticator))
	body := map[string]any{
		"name": name,
		"federatedAuthenticators": map[string]any{
			"defaultAuthenticatorId": authID,
			"authenticators": []any{map[string]any{
				"authenticatorId": authID,
				"isEnabled":       true,
				"properties":      props,
			}},
		},
	}
	toolkit.PutIf(body, args, map[string]string{"description": "description"})
	return body, nil
}

// commonAuthURL is the IS endpoint external providers redirect back to.
func commonAuthURL(cfg *config.Config) string {
	base := strings.TrimRight(cfg.ServerURL, "/")
	if cfg.TenantDomain != "" && cfg.TenantDomain != config.DefaultTenantDomain {
		base += "/t/" + cfg.TenantDomain
	}
	return base + "/commonauth"
}
