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

package applications

import (
	"context"
	"fmt"
	"time"

	"github.com/asgardeo/mcp/internal/tools/toolkit"
)

// Login flow generation polling (the Asgardeo SDK tool polled every 2 s without a limit).
const (
	flowPollInterval = 2 * time.Second
	flowPollTimeout  = 3 * time.Minute
)

// Authenticator ids (base64url of the authenticator names) the login flow AI groups apart.
const backupCodeAuthenticatorID = "YmFja3VwLWNvZGUtYXV0aGVudGljYXRvcg"

var secondFactorAuthenticatorIDs = map[string]bool{
	"dG90cA":                          true, // TOTP
	"SXByb292QXV0aGVudGljYXRvcg":      true, // iProov
	"RHVvQXV0aGVudGljYXRvcg":          true, // Duo
	"cGFzc3dvcmQtcmVzZXQtZW5mb3JjZXI": true, // password reset enforcer
}

var socialAuthenticatorIDs = map[string]bool{
	"QXBwbGVPSURDQXV0aGVudGljYXRvcg":  true, // Apple
	"R29vZ2xlT0lEQ0F1dGhlbnRpY2F0b3I": true, // Google
	"RmFjZWJvb2tBdXRoZW50aWNhdG9y":    true, // Facebook
	"VHdpdHRlckF1dGhlbnRpY2F0b3I":     true, // Twitter
	"R2l0aHViQXV0aGVudGljYXRvcg":      true, // GitHub
}

// updateLoginFlow asks the IS login flow AI for an authentication sequence that matches the
// prompt, waits for it, and patches the application with it.
func (a *apps) updateLoginFlow(ctx context.Context, args map[string]any) (any, error) {
	appID, err := toolkit.RequireString(args, "app_id")
	if err != nil {
		return nil, err
	}
	prompt, err := toolkit.RequireString(args, "user_prompt")
	if err != nil {
		return nil, err
	}
	authenticators, err := a.availableAuthenticators(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := a.userClaims(ctx)
	if err != nil {
		return nil, err
	}
	gen, err := toolkit.Call(ctx, a.cl, a.o.flowGenerate, nil, nil, map[string]any{
		"availableAuthenticators": authenticators, "userClaims": claims, "userQuery": prompt,
	})
	if err != nil {
		return nil, err
	}
	flowID, _ := asMap(gen)["operation_id"].(string)
	if flowID == "" {
		return nil, fmt.Errorf("login flow generation returned no operation_id")
	}
	path := map[string]string{"id": flowID}

	deadline := time.Now().Add(flowPollTimeout)
	for {
		st, err := toolkit.Call(ctx, a.cl, a.o.flowStatus, path, nil, nil)
		if err != nil {
			return nil, err
		}
		if allTrue(asMap(asMap(st)["status"])) {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("login flow generation %s did not finish within %s", flowID, flowPollTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(flowPollInterval):
		}
	}

	res, err := toolkit.Call(ctx, a.cl, a.o.flowResult, path, nil, nil)
	if err != nil {
		return nil, err
	}
	sequence := asMap(res)["data"]
	if sequence == nil {
		return nil, fmt.Errorf("login flow generation %s returned no authentication sequence", flowID)
	}
	body := map[string]any{"authenticationSequence": sequence}
	if _, err := toolkit.Call(ctx, a.cl, a.o.patch, map[string]string{"applicationId": appID}, nil, body); err != nil {
		return nil, err
	}
	return "Login flow generated successfully.", nil
}

func allTrue(status map[string]any) bool {
	if len(status) == 0 {
		return false
	}
	for _, v := range status {
		if b, _ := v.(bool); !b {
			return false
		}
	}
	return true
}

// availableAuthenticators groups the local authenticators and the identity providers the way
// the login flow AI expects.
func (a *apps) availableAuthenticators(ctx context.Context) (map[string]any, error) {
	local, second, recovery, enterprise, social := []any{}, []any{}, []any{}, []any{}, []any{}

	resp, err := toolkit.Call(ctx, a.cl, a.o.authenticators, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	for _, it := range asSlice(resp) {
		m := asMap(it)
		if m["type"] != "LOCAL" {
			continue
		}
		desc, _ := m["description"].(string)
		entry := map[string]any{"description": desc, "idp": m["type"], "name": m["name"]}
		id, _ := m["id"].(string)
		switch {
		case id == backupCodeAuthenticatorID:
			recovery = append(recovery, entry)
		case secondFactorAuthenticatorIDs[id]:
			second = append(second, entry)
		default:
			local = append(local, entry)
		}
	}

	idps, err := toolkit.Call(ctx, a.cl, a.o.idps, nil, map[string]any{"requiredAttributes": "federatedAuthenticators"}, nil)
	if err != nil {
		return nil, err
	}
	for _, it := range asSlice(asMap(idps)["identityProviders"]) {
		idp := asMap(it)
		fa := asMap(idp["federatedAuthenticators"])
		defaultID, _ := fa["defaultAuthenticatorId"].(string)
		name, _ := idp["name"].(string)
		if name == "" || defaultID == "" {
			continue
		}
		authName := ""
		for _, au := range asSlice(fa["authenticators"]) {
			if am := asMap(au); am["authenticatorId"] == defaultID {
				authName, _ = am["name"].(string)
				break
			}
		}
		if authName == "" {
			continue
		}
		desc, _ := idp["description"].(string)
		entry := map[string]any{"description": desc, "idp": name, "name": authName}
		if socialAuthenticatorIDs[defaultID] {
			social = append(social, entry)
		} else {
			enterprise = append(enterprise, entry)
		}
	}
	return map[string]any{
		"enterprise": enterprise, "local": local, "recovery": recovery, "secondFactor": second, "social": social,
	}, nil
}

// userClaims lists the visible local claims with their descriptions.
func (a *apps) userClaims(ctx context.Context) ([]any, error) {
	resp, err := toolkit.Call(ctx, a.cl, a.o.localClaims, nil, map[string]any{"exclude-hidden-claims": true}, nil)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, it := range asSlice(resp) {
		m := asMap(it)
		uri, _ := m["claimURI"].(string)
		desc, _ := m["description"].(string)
		if uri != "" && desc != "" {
			out = append(out, map[string]any{"claimURI": uri, "description": desc})
		}
	}
	return out, nil
}
