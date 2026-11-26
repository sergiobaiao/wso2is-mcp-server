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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/asgardeo/mcp/internal/config"
)

const (
	tokenExpiryMargin = 30 * time.Second
	maxTokenBodyBytes = 1 << 20 // 1 MiB
)

type tokenEntry struct {
	accessToken string
	expiresAt   time.Time
}

type tokenCache struct {
	mu        sync.Mutex
	rootToken *tokenEntry
	orgTokens map[string]tokenEntry
	now       func() time.Time
}

func newTokenCache() *tokenCache {
	return &tokenCache{
		orgTokens: make(map[string]tokenEntry),
		now:       time.Now,
	}
}

func (tc *tokenCache) isValid(entry tokenEntry) bool {
	if entry.accessToken == "" {
		return false
	}
	return tc.now().Add(tokenExpiryMargin).Before(entry.expiresAt)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (c *Client) resolveToken(ctx context.Context, scopes []string) (string, error) {
	cc, _ := CredentialContextFrom(ctx)
	mode := cc.Mode
	if mode == "" {
		mode = c.cfg.AuthMode
	}

	var rootToken string
	var err error
	switch mode {
	case config.AuthModeClientCredentials:
		rootToken, err = c.getClientCredentialsToken(ctx)
		if err != nil {
			return "", err
		}
	case config.AuthModeStaticToken:
		if c.cfg.AccessToken == "" {
			return "", errors.New("static_token auth mode requires ACCESS_TOKEN")
		}
		rootToken = c.cfg.AccessToken
	case config.AuthModeBearer:
		if cc.BearerToken == "" {
			return "", errors.New("bearer auth mode requires a bearer token in CredentialContext")
		}
		rootToken = cc.BearerToken
	default:
		return "", fmt.Errorf("unsupported auth mode %q", mode)
	}

	if cc.OrganizationID == "" {
		return rootToken, nil
	}
	return c.getOrgSwitchToken(ctx, rootToken, cc.OrganizationID, scopes)
}

func (c *Client) getClientCredentialsToken(ctx context.Context) (string, error) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	if c.tokens.rootToken != nil && c.tokens.isValid(*c.tokens.rootToken) {
		return c.tokens.rootToken.accessToken, nil
	}
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return "", errors.New("client_credentials auth mode requires CLIENT_ID and CLIENT_SECRET")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "SYSTEM")

	entry, err := c.fetchToken(ctx, form)
	if err != nil {
		return "", err
	}
	c.tokens.rootToken = &entry
	return entry.accessToken, nil
}

func (c *Client) getOrgSwitchToken(ctx context.Context, rootToken, orgID string, scopes []string) (string, error) {
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return "", errors.New("organization_id requires CLIENT_ID and CLIENT_SECRET to be configured for organization_switch")
	}

	sum := sha256.Sum256([]byte(rootToken))
	scopeStr := strings.Join(scopes, " ")
	key := hex.EncodeToString(sum[:]) + ":" + orgID
	if scopeStr != "" {
		key += ":" + scopeStr
	}

	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	if entry, ok := c.tokens.orgTokens[key]; ok && c.tokens.isValid(entry) {
		return entry.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "organization_switch")
	form.Set("token", rootToken)
	form.Set("switching_organization", orgID)
	if scopeStr != "" {
		form.Set("scope", scopeStr)
	}

	entry, err := c.fetchToken(ctx, form)
	if err != nil {
		return "", err
	}
	c.tokens.orgTokens[key] = entry
	return entry.accessToken, nil
}

func (c *Client) fetchToken(ctx context.Context, form url.Values) (tokenEntry, error) {
	tokenURL := strings.TrimRight(c.cfg.TenantBaseURL(), "/") + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenEntry{}, fmt.Errorf("create token request: %w", err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tokenEntry{}, fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenBodyBytes))
	if err != nil {
		return tokenEntry{}, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenEntry{}, newAPIError(resp.StatusCode, body, nil)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return tokenEntry{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return tokenEntry{}, errors.New("token response missing access_token")
	}

	now := c.tokens.now()
	var expiresAt time.Time
	if tr.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tokenEntry{
		accessToken: tr.AccessToken,
		expiresAt:   expiresAt,
	}, nil
}
