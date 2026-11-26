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

// Package applications implements the application tools: the 13 tools migrated from the
// Asgardeo SDK (names and inputs frozen, FR-009) plus delete_application and share_application
// (contracts/admin-curated-tools.md).
package applications

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Application types and the IS template ids that identify them.
const (
	typeSPA    = "spa"
	typeMobile = "mobile"
	typeM2M    = "m2m"
	typeSSRWeb = "ssr_web"
)

var templateTypes = map[string]string{
	"6a90e4b0-fbff-42d7-bfde-1efd98f07cd7": typeSPA,
	"mobile-application":                   typeMobile,
	"m2m-application":                      typeM2M,
	"b9c5e11e-fc78-484b-9bec-015d247561b8": typeSSRWeb,
}

// oidcDialectID is the base64url id of the http://wso2.org/oidc/claim dialect.
const oidcDialectID = "aHR0cDovL3dzbzIub3JnL29pZGMvY2xhaW0"

type ops struct {
	list, create, get, patch, del, oidcGet, oidcPut, authAdd, authList, share toolkit.OpPair
	oidcScopes, oidcClaims, authenticators, idps, localClaims                 toolkit.OpPair
	flowGenerate, flowStatus, flowResult                                      toolkit.OpPair
}

func lookup(cat *catalog.Catalog) (*ops, error) {
	o := &ops{}
	for _, e := range []struct {
		dst         *toolkit.OpPair
		root, orgID string
	}{
		{&o.list, "applications.getAllApplications", "applications.org.getAllApplications"},
		{&o.create, "applications.createApplication", "applications.org.createApplication"},
		{&o.get, "applications.getApplication", "applications.org.getApplication"},
		{&o.patch, "applications.patchApplication", "applications.org.patchApplication"},
		{&o.del, "applications.deleteApplication", "applications.org.deleteApplication"},
		{&o.oidcGet, "applications.getInboundOAuthConfiguration", "applications.org.getInboundOAuthConfiguration"},
		{&o.oidcPut, "applications.updateInboundOAuthConfiguration", "applications.org.updateInboundOAuthConfiguration"},
		{&o.authAdd, "applications.addAuthorizedAPI", "applications.org.addAuthorizedAPI"},
		{&o.authList, "applications.getAuthorizedAPIs", "applications.org.getAuthorizedAPIs"},
		{&o.share, "applications.shareOrgApplication", ""},
		{&o.oidcScopes, "api_authorization.getApiServerV1OidcScopes", ""},
		{&o.oidcClaims, "claims.getExternalClaims", "claims.org.getExternalClaims"},
		{&o.authenticators, "identity_providers.getAuthenticators", ""},
		{&o.idps, "identity_providers.getIDPs", ""},
		{&o.localClaims, "claims.getLocalClaims", ""},
		{&o.flowGenerate, "applications.generateLoginFlow", ""},
		{&o.flowStatus, "applications.getLoginFlowStatus", ""},
		{&o.flowResult, "applications.getLoginFlowResult", ""},
	} {
		p, err := toolkit.LookupPair(cat, e.root, e.orgID)
		if err != nil {
			return nil, err
		}
		*e.dst = p
	}
	return o, nil
}

// Tools returns the 15 application tools.
func Tools(cat *catalog.Catalog, cl *client.Client, cfg *config.Config) ([]toolkit.ToolDef, error) {
	o, err := lookup(cat)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	a := &apps{o: o, cl: cl, cfg: cfg}

	appName := mcp.WithString("application_name", mcp.Description("Name of the application"), mcp.Required())
	redirectURL := mcp.WithString("redirect_url", mcp.Description("Redirect URL of the application"), mcp.Required())
	appID := mcp.WithString("id", mcp.Description("ID of the application"), mcp.Required())

	def := func(name string, kind catalog.Kind, org bool, desc string, main toolkit.OpPair, extra []*catalog.Operation,
		opts []mcp.ToolOption, h func(context.Context, map[string]any) (any, error)) toolkit.ToolDef {
		return toolkit.ToolDef{
			Name: name, Layer: toolkit.LayerCurated, Kind: kind, OrgCapable: org,
			Scopes:    toolkit.UnionScopes(append([]*catalog.Operation{main.Root, main.Org}, extra...)...),
			Operation: main.Root, Description: desc, Options: opts, Handler: h,
		}
	}
	createTool := func(name, appType, desc string, withRedirect bool) toolkit.ToolDef {
		opts := []mcp.ToolOption{appName}
		if withRedirect {
			opts = append(opts, redirectURL)
		}
		return def(name, catalog.KindWrite, true, desc, o.create,
			[]*catalog.Operation{o.list.Root, o.list.Org, o.get.Root, o.get.Org, o.oidcGet.Root, o.oidcGet.Org}, opts,
			func(ctx context.Context, args map[string]any) (any, error) { return a.createApp(ctx, args, appType) })
	}

	return []toolkit.ToolDef{
		def("list_applications", catalog.KindRead, true, "List all applications in WSO2 Identity Server.", o.list, nil, nil,
			a.listApps),
		createTool("create_single_page_app", typeSPA, "Create a new Single Page Application in WSO2 Identity Server.", true),
		createTool("create_webapp_with_ssr", typeSSRWeb, "Create a new regular web application that implements server side rendering in WSO2 Identity Server.", true),
		createTool("create_mobile_app", typeMobile, "Create a new Mobile Application in WSO2 Identity Server.", true),
		createTool("create_m2m_app", typeM2M, "Create a new M2M Application in WSO2 Identity Server.", false),
		def("get_application_by_name", catalog.KindRead, true, "Get details of an application by name in WSO2 Identity Server.", o.list,
			[]*catalog.Operation{o.get.Root, o.get.Org, o.oidcGet.Root, o.oidcGet.Org, o.authList.Root, o.authList.Org, o.oidcScopes.Root, o.oidcClaims.Root},
			[]mcp.ToolOption{appName}, func(ctx context.Context, args map[string]any) (any, error) {
				name, err := toolkit.RequireString(args, "application_name")
				if err != nil {
					return nil, err
				}
				return a.describe(ctx, "name", name)
			}),
		def("get_application_by_client_id", catalog.KindRead, true, "Get details of an application by client ID in WSO2 Identity Server.", o.list,
			[]*catalog.Operation{o.get.Root, o.get.Org, o.oidcGet.Root, o.oidcGet.Org, o.authList.Root, o.authList.Org, o.oidcScopes.Root, o.oidcClaims.Root},
			[]mcp.ToolOption{mcp.WithString("client_id", mcp.Description("Client ID of the application"), mcp.Required())},
			func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "client_id")
				if err != nil {
					return nil, err
				}
				return a.describe(ctx, "clientId", id)
			}),
		def("update_application_basic_info", catalog.KindWrite, true, "Update basic information of an application in WSO2 Identity Server.", o.patch, nil,
			[]mcp.ToolOption{appID,
				mcp.WithString("name", mcp.Description("Name of the application")),
				mcp.WithString("description", mcp.Description("Description of the application")),
				mcp.WithString("image_url", mcp.Description("URL of the application image icon")),
				mcp.WithString("access_url", mcp.Description("Access URL of the application")),
				mcp.WithString("logout_return_url", mcp.Description("A URL of the application to return upon logout")),
			}, a.updateBasicInfo),
		def("update_application_oauth_config", catalog.KindWrite, true, "Update OAuth/OIDC configurations of an application in WSO2 Identity Server.", o.oidcPut,
			[]*catalog.Operation{o.oidcGet.Root, o.oidcGet.Org},
			[]mcp.ToolOption{appID,
				mcp.WithArray("redirect_urls", mcp.Description("Redirect URLs of the application"), mcp.WithStringItems()),
				mcp.WithNumber("user_access_token_expiry_time", mcp.Description("Expiry time in seconds of the access token issued on behalf of the user")),
				mcp.WithNumber("application_access_token_expiry_time", mcp.Description("Expiry time in seconds of the access token issued on behalf of the application")),
				mcp.WithNumber("refresh_token_expiry_time", mcp.Description("Expiry time in seconds of the refresh token")),
				mcp.WithArray("allowed_origins", mcp.Description("Allowed origins for CORS"), mcp.WithStringItems()),
				mcp.WithBoolean("revoke_tokens_when_idp_session_terminated", mcp.Description("Revoke tokens when IDP session is terminated")),
				mcp.WithArray("access_token_attributes", mcp.Description("Access token attributes"), mcp.WithStringItems()),
			}, a.updateOAuthConfig),
		def("update_application_claim_config", catalog.KindWrite, true, "Update claim configurations of an application in WSO2 Identity Server.", o.patch, nil,
			[]mcp.ToolOption{appID,
				mcp.WithArray("claims", mcp.Required(), mcp.WithStringItems(),
					mcp.Description("Claim URIs to add as requested claims, e.g. http://wso2.org/claims/username, http://wso2.org/claims/emailaddress")),
			}, a.updateClaimConfig),
		def("authorize_api", catalog.KindWrite, true, "Authorize an API resource for an application in WSO2 Identity Server.", o.authAdd, nil,
			[]mcp.ToolOption{
				mcp.WithString("appId", mcp.Required(), mcp.Description("ID of the application.")),
				mcp.WithString("id", mcp.Required(), mcp.Description("ID of the API resource to authorize.")),
				mcp.WithString("policyIdentifier", mcp.Required(), mcp.DefaultString("RBAC"), mcp.Description("Authorization policy of the API authorization.")),
				mcp.WithArray("scopes", mcp.Required(), mcp.WithStringItems(), mcp.Description("Scope names of the API resource to authorize.")),
			}, a.authorizeAPI),
		def("list_authorized_api", catalog.KindRead, true, "List authorized API resources of an application in WSO2 Identity Server.", o.authList, nil,
			[]mcp.ToolOption{mcp.WithString("app_id", mcp.Required(), mcp.Description("ID of the application."))},
			a.listAuthorizedAPIs),
		def("update_login_flow", catalog.KindWrite, false, "Update the login flow of an application in WSO2 Identity Server from a natural language prompt (uses the IS login flow AI feature).", o.patch,
			[]*catalog.Operation{o.flowGenerate.Root, o.flowStatus.Root, o.flowResult.Root, o.authenticators.Root, o.idps.Root, o.localClaims.Root},
			[]mcp.ToolOption{
				mcp.WithString("app_id", mcp.Required(), mcp.Description("ID of the application whose login flow is updated.")),
				mcp.WithString("user_prompt", mcp.Required(), mcp.Description(`The login flow wanted, e.g. "Username and password as first factor and Email OTP as second factor".`)),
			}, a.updateLoginFlow),
		def("delete_application", catalog.KindDestructive, true, "Delete an application from WSO2 Identity Server. Its client credentials stop working.", o.del, nil,
			[]mcp.ToolOption{appID}, func(ctx context.Context, args map[string]any) (any, error) {
				id, err := toolkit.RequireString(args, "id")
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, o.del, map[string]string{"applicationId": id}, nil, nil)
			}),
		def("share_application", catalog.KindWrite, false, "Share an application of the root organization of WSO2 Identity Server with all child organizations or with listed ones.", o.share, nil,
			[]mcp.ToolOption{appID,
				mcp.WithBoolean("share_with_all_children", mcp.Required(), mcp.Description("Share with all current and future child organizations.")),
				mcp.WithArray("shared_organizations", mcp.WithStringItems(), mcp.Description("Organization ids to share with when share_with_all_children is false.")),
			}, a.shareApp),
	}, nil
}

type apps struct {
	o   *ops
	cl  *client.Client
	cfg *config.Config
}

func (a *apps) listApps(ctx context.Context, _ map[string]any) (any, error) {
	resp, err := toolkit.CallDeclared(ctx, a.cl, a.o.list, nil, map[string]any{"limit": 10, "offset": 0, "excludeSystemPortals": true}, nil)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, it := range items(resp, "applications") {
		out = append(out, map[string]any{"name": it["name"], "id": it["id"]})
	}
	return out, nil
}

// findApp returns the list entry whose field equals value.
func (a *apps) findApp(ctx context.Context, field, value string) (map[string]any, error) {
	q := map[string]any{"filter": field + " eq " + value, "excludeSystemPortals": true, "attributes": "templateId,clientId"}
	resp, err := toolkit.CallDeclared(ctx, a.cl, a.o.list, nil, q, nil)
	if err != nil {
		return nil, err
	}
	for _, it := range items(resp, "applications") {
		if s, _ := it[field].(string); s == value {
			return it, nil
		}
	}
	return nil, fmt.Errorf("application with %s %q not found", field, value)
}

// createApp creates the application from the type's defaults (the bodies the Asgardeo SDK sent).
// ponytail: the new id comes from a name lookup (names are unique per organization), because
// the client does not expose the Location header.
func (a *apps) createApp(ctx context.Context, args map[string]any, appType string) (any, error) {
	name, err := toolkit.RequireString(args, "application_name")
	if err != nil {
		return nil, err
	}
	redirect := ""
	if appType != typeM2M {
		if redirect, err = toolkit.RequireString(args, "redirect_url"); err != nil {
			return nil, err
		}
	}
	body, err := createBody(appType, name, redirect)
	if err != nil {
		return nil, err
	}
	if _, err := toolkit.Call(ctx, a.cl, a.o.create, nil, nil, body); err != nil {
		return nil, err
	}
	created, err := a.findApp(ctx, "name", name)
	if err != nil {
		return nil, fmt.Errorf("created application %q but could not read it back: %w", name, err)
	}
	id, _ := created["id"].(string)
	path := map[string]string{"applicationId": id}

	conf := map[string]any{"name": name, "id": id, "application_type": appType}
	if appType == typeSPA || appType == typeMobile {
		details, err := toolkit.Call(ctx, a.cl, a.o.get, path, nil, nil)
		if err != nil {
			return nil, err
		}
		conf["client_id"] = asMap(details)["clientId"]
		conf["redirect_url"] = redirect
		conf["scope"] = "openid profile email"
	} else {
		oauth, err := toolkit.Call(ctx, a.cl, a.o.oidcGet, path, nil, nil)
		if err != nil {
			return nil, err
		}
		conf["client_id"] = asMap(oauth)["clientId"]
		conf["scope"] = "openid profile email"
		if appType == typeSSRWeb {
			conf["redirect_url"] = redirect
		}
	}
	return map[string]any{"application_configurations": conf, "oauth_endpoints": a.endpoints(appType)}, nil
}

// endpoints are the OAuth endpoints of the tenant (SERVER_URL and TENANT_DOMAIN).
func (a *apps) endpoints(appType string) map[string]any {
	base := strings.TrimRight(a.cfg.ServerURL, "/")
	if a.cfg.TenantDomain != "" && a.cfg.TenantDomain != config.DefaultTenantDomain {
		base += "/t/" + a.cfg.TenantDomain
	}
	ep := map[string]any{
		"token_url":    base + "/oauth2/token",
		"jwks_url":     base + "/oauth2/jwks",
		"userinfo_url": base + "/oauth2/userinfo",
	}
	if appType != typeM2M {
		ep["base_url"] = base
		ep["authorize_url"] = base + "/oauth2/authorize"
	}
	return ep
}

// describe returns the legacy get_application_by_* output for the application whose field
// equals value.
func (a *apps) describe(ctx context.Context, field, value string) (any, error) {
	found, err := a.findApp(ctx, field, value)
	if err != nil {
		return nil, err
	}
	id, _ := found["id"].(string)
	path := map[string]string{"applicationId": id}
	details, err := toolkit.Call(ctx, a.cl, a.o.get, path, nil, nil)
	if err != nil {
		return nil, err
	}
	app := asMap(details)
	oauthResp, err := toolkit.Call(ctx, a.cl, a.o.oidcGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	oauth := asMap(oauthResp)
	authResp, err := toolkit.Call(ctx, a.cl, a.o.authList, path, nil, nil)
	if err != nil {
		return nil, err
	}

	var apiScopes []string
	seen := map[string]bool{}
	for _, api := range asSlice(authResp) {
		for _, s := range asSlice(asMap(api)["authorizedScopes"]) {
			if n, _ := asMap(s)["name"].(string); n != "" && !seen[n] {
				seen[n] = true
				apiScopes = append(apiScopes, n)
			}
		}
	}
	sort.Strings(apiScopes)

	tpl, _ := app["templateId"].(string)
	appType := templateTypes[tpl]
	if appType == "" {
		return nil, fmt.Errorf("application %q has an unknown application type (template %q)", value, tpl)
	}
	conf := map[string]any{"name": app["name"], "id": id, "client_id": app["clientId"], "application_type": appType}
	if conf["client_id"] == nil {
		conf["client_id"] = oauth["clientId"]
	}
	if appType == typeM2M {
		conf["scope"] = strings.Join(apiScopes, " ")
	} else {
		if cbs := asSlice(oauth["callbackURLs"]); len(cbs) > 0 {
			first, _ := cbs[0].(string)
			if strings.HasPrefix(first, "regexp=") {
				first = strings.Join(strings.Split(strings.Trim(strings.TrimPrefix(first, "regexp="), "()"), "|"), ",")
			}
			conf["redirect_url"] = first
		}
		oidcScopes, err := a.authorizedOIDCScopes(ctx, asMap(app["claimConfiguration"]))
		if err != nil {
			return nil, err
		}
		conf["scope"] = strings.Join(append(oidcScopes, apiScopes...), " ")
	}
	return map[string]any{"application_configurations": conf, "oauth_endpoints": a.endpoints(appType)}, nil
}

// authorizedOIDCScopes returns openid plus the OIDC scopes whose claims map to the requested
// local claims. Outside the root organization IS has no OIDC scope API, so only openid is
// returned there.
func (a *apps) authorizedOIDCScopes(ctx context.Context, claimConfig map[string]any) ([]string, error) {
	requested := asSlice(claimConfig["requestedClaims"])
	if len(requested) == 0 {
		return []string{"openid"}, nil
	}
	if _, err := a.o.oidcScopes.Select(ctx); err != nil {
		return []string{"openid"}, nil
	}
	scopesResp, err := toolkit.Call(ctx, a.cl, a.o.oidcScopes, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	claimsResp, err := toolkit.Call(ctx, a.cl, a.o.oidcClaims, map[string]string{"dialect-id": oidcDialectID}, nil, nil)
	if err != nil {
		return nil, err
	}
	localToOIDC := map[string]string{}
	for _, c := range asSlice(claimsResp) {
		m := asMap(c)
		local, _ := m["mappedLocalClaimURI"].(string)
		oidc, _ := m["claimURI"].(string)
		if local != "" && oidc != "" {
			localToOIDC[local] = oidc
		}
	}
	found := map[string]bool{}
	for _, rc := range requested {
		uri, _ := asMap(asMap(rc)["claim"])["uri"].(string)
		oidcClaim, ok := localToOIDC[uri]
		if !ok {
			continue
		}
		for _, s := range asSlice(scopesResp) {
			sm := asMap(s)
			name, _ := sm["name"].(string)
			for _, c := range asSlice(sm["claims"]) {
				if c == oidcClaim && name != "openid" {
					found[name] = true
				}
			}
		}
	}
	out := make([]string, 0, len(found))
	for s := range found {
		out = append(out, s)
	}
	sort.Strings(out)
	return append([]string{"openid"}, out...), nil
}

func (a *apps) updateBasicInfo(ctx context.Context, args map[string]any) (any, error) {
	id, err := toolkit.RequireString(args, "id")
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	toolkit.PutIf(body, args, map[string]string{
		"name": "name", "description": "description", "image_url": "imageUrl",
		"access_url": "accessUrl", "logout_return_url": "logoutReturnUrl",
	})
	if _, err := toolkit.Call(ctx, a.cl, a.o.patch, map[string]string{"applicationId": id}, nil, body); err != nil {
		return nil, err
	}
	return "Successfully updated the application.", nil
}

func (a *apps) updateOAuthConfig(ctx context.Context, args map[string]any) (any, error) {
	id, err := toolkit.RequireString(args, "id")
	if err != nil {
		return nil, err
	}
	path := map[string]string{"applicationId": id}
	current, err := toolkit.Call(ctx, a.cl, a.o.oidcGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	conf := asMap(current)
	at := asMap(conf["accessToken"])
	for arg, key := range map[string]string{
		"user_access_token_expiry_time":             "userAccessTokenExpiryInSeconds",
		"application_access_token_expiry_time":      "applicationAccessTokenExpiryInSeconds",
		"revoke_tokens_when_idp_session_terminated": "revokeTokensWhenIDPSessionTerminated",
		"access_token_attributes":                   "accessTokenAttributes",
	} {
		if v, ok := args[arg]; ok && v != nil {
			at[key] = v
		}
	}
	conf["accessToken"] = at
	if v, ok := args["refresh_token_expiry_time"]; ok && v != nil {
		rt := asMap(conf["refreshToken"])
		rt["expiryInSeconds"] = v
		conf["refreshToken"] = rt
	}
	if v, ok := args["allowed_origins"]; ok && v != nil {
		conf["allowedOrigins"] = v
	}
	urls, err := toolkit.OptStrings(args, "redirect_urls")
	if err != nil {
		return nil, err
	}
	switch {
	case len(urls) == 1:
		conf["callbackURLs"] = urls
	case len(urls) > 1:
		conf["callbackURLs"] = []string{"regexp=(" + strings.Join(urls, "|") + ")"}
	}
	delete(conf, "clientSecret") // never echo the secret back; IS keeps the stored one
	if _, err := toolkit.Call(ctx, a.cl, a.o.oidcPut, path, nil, conf); err != nil {
		return nil, err
	}
	return "Successfully updated the application.", nil
}

func (a *apps) updateClaimConfig(ctx context.Context, args map[string]any) (any, error) {
	id, err := toolkit.RequireString(args, "id")
	if err != nil {
		return nil, err
	}
	uris, err := toolkit.OptStrings(args, "claims")
	if err != nil {
		return nil, err
	}
	if len(uris) == 0 {
		return nil, fmt.Errorf("invalid input: claims: is required")
	}
	requested := make([]any, 0, len(uris))
	for _, u := range uris {
		requested = append(requested, map[string]any{"claim": map[string]any{"uri": u}, "mandatory": false})
	}
	body := map[string]any{"claimConfiguration": map[string]any{"requestedClaims": requested}}
	if _, err := toolkit.Call(ctx, a.cl, a.o.patch, map[string]string{"applicationId": id}, nil, body); err != nil {
		return nil, err
	}
	return "Successfully updated the claim configuration of the application.", nil
}

func (a *apps) authorizeAPI(ctx context.Context, args map[string]any) (any, error) {
	appID, err := toolkit.RequireString(args, "appId")
	if err != nil {
		return nil, err
	}
	apiID, err := toolkit.RequireString(args, "id")
	if err != nil {
		return nil, err
	}
	policy, err := toolkit.OptString(args, "policyIdentifier")
	if err != nil {
		return nil, err
	}
	if policy == "" {
		policy = "RBAC"
	}
	scopes, err := toolkit.OptStrings(args, "scopes")
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = []string{}
	}
	body := map[string]any{"id": apiID, "policyIdentifier": policy, "scopes": scopes}
	if _, err := toolkit.Call(ctx, a.cl, a.o.authAdd, map[string]string{"applicationId": appID}, nil, body); err != nil {
		return nil, err
	}
	return "API authorization successful.", nil
}

func (a *apps) listAuthorizedAPIs(ctx context.Context, args map[string]any) (any, error) {
	appID, err := toolkit.RequireString(args, "app_id")
	if err != nil {
		return nil, err
	}
	resp, err := toolkit.Call(ctx, a.cl, a.o.authList, map[string]string{"applicationId": appID}, nil, nil)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, it := range asSlice(resp) {
		api := asMap(it)
		scopes := []any{}
		for _, s := range asSlice(api["authorizedScopes"]) {
			sm := asMap(s)
			scopes = append(scopes, map[string]any{"id": sm["id"], "name": sm["name"], "display_name": sm["displayName"]})
		}
		out = append(out, map[string]any{
			"id": api["id"], "identifier": api["identifier"], "display_name": api["displayName"],
			"policy_id": api["policyId"], "type": api["type"], "authorized_scopes": scopes,
		})
	}
	return out, nil
}

func (a *apps) shareApp(ctx context.Context, args map[string]any) (any, error) {
	id, err := toolkit.RequireString(args, "id")
	if err != nil {
		return nil, err
	}
	all, ok, err := toolkit.OptBool(args, "share_with_all_children")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invalid input: share_with_all_children: is required")
	}
	orgs, err := toolkit.OptStrings(args, "shared_organizations")
	if err != nil {
		return nil, err
	}
	if !all && len(orgs) == 0 {
		return nil, fmt.Errorf("invalid input: shared_organizations: is required when share_with_all_children is false")
	}
	body := map[string]any{"shareWithAllChildren": all}
	if len(orgs) > 0 {
		body["sharedOrganizations"] = orgs
	}
	return toolkit.Call(ctx, a.cl, a.o.share, map[string]string{"applicationId": id}, nil, body)
}

// createBody returns the createApplication body for the application type, as the Asgardeo SDK
// built it.
func createBody(appType, name, redirect string) (map[string]any, error) {
	basicLogin := map[string]any{
		"type":  "DEFAULT",
		"steps": []any{map[string]any{"id": 1, "options": []any{map[string]any{"idp": "LOCAL", "authenticator": "basic"}}}},
	}
	advanced := map[string]any{"discoverableByEndUsers": false, "skipLogoutConsent": true, "skipLoginConsent": true}
	usernameClaim := map[string]any{
		"dialect":         "LOCAL",
		"requestedClaims": []any{map[string]any{"claim": map[string]any{"uri": "http://wso2.org/claims/username"}}},
	}
	roles := map[string]any{"allowedAudience": "APPLICATION", "roles": []any{}}
	pkce := map[string]any{"mandatory": true, "supportPlainTransformAlgorithm": false}
	refresh := map[string]any{"expiryInSeconds": 86400, "renewRefreshToken": true}
	accessToken := func(binding string, revoke bool) map[string]any {
		return map[string]any{
			"applicationAccessTokenExpiryInSeconds": 3600, "userAccessTokenExpiryInSeconds": 3600,
			"bindingType": binding, "revokeTokensWhenIDPSessionTerminated": revoke, "type": "JWT",
			"validateTokenBinding": false,
		}
	}

	switch appType {
	case typeSPA:
		u, err := url.Parse(redirect)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid input: redirect_url: must be an absolute URL")
		}
		return map[string]any{
			"name": name, "templateId": "6a90e4b0-fbff-42d7-bfde-1efd98f07cd7", "advancedConfigurations": advanced,
			"authenticationSequence": basicLogin, "claimConfiguration": usernameClaim, "associatedRoles": roles,
			"inboundProtocolConfiguration": map[string]any{"oidc": map[string]any{
				"grantTypes": []string{"authorization_code", "refresh_token"}, "callbackURLs": []string{redirect},
				"allowedOrigins": []string{u.Scheme + "://" + u.Host}, "publicClient": true, "pkce": pkce,
				"accessToken": accessToken("sso-session", true), "refreshToken": refresh,
			}},
		}, nil
	case typeMobile:
		return map[string]any{
			"name": name, "templateId": "mobile-application", "advancedConfigurations": advanced,
			"authenticationSequence": basicLogin, "associatedRoles": roles,
			"inboundProtocolConfiguration": map[string]any{"oidc": map[string]any{
				"grantTypes": []string{"authorization_code", "refresh_token"}, "callbackURLs": []string{redirect},
				"allowedOrigins": []string{}, "publicClient": true, "pkce": pkce,
				"accessToken": accessToken("None", false), "refreshToken": refresh,
			}},
		}, nil
	case typeSSRWeb:
		at := accessToken("cookie", true)
		delete(at, "validateTokenBinding")
		return map[string]any{
			"name": name, "templateId": "b9c5e11e-fc78-484b-9bec-015d247561b8", "advancedConfigurations": advanced,
			"authenticationSequence": basicLogin, "claimConfiguration": usernameClaim, "associatedRoles": roles,
			"inboundProtocolConfiguration": map[string]any{"oidc": map[string]any{
				"grantTypes": []string{"authorization_code"}, "callbackURLs": []string{redirect},
				"allowedOrigins": []string{}, "publicClient": false,
				"accessToken": at, "refreshToken": map[string]any{"expiryInSeconds": 86400},
			}},
		}, nil
	default: // typeM2M
		return map[string]any{
			"name": name, "templateId": "m2m-application", "authenticationSequence": map[string]any{"type": "DEFAULT"},
			"associatedRoles": roles,
			"inboundProtocolConfiguration": map[string]any{"oidc": map[string]any{
				"grantTypes": []string{"client_credentials"}, "publicClient": false,
			}},
		}, nil
	}
}

func items(resp any, key string) []map[string]any {
	var out []map[string]any
	for _, it := range asSlice(asMap(resp)[key]) {
		out = append(out, asMap(it))
	}
	return out
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
