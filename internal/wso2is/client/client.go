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

// Package client implements the shared WSO2 Identity Server HTTP client,
// token management, request validation, error formatting, and response truncation.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/getkin/kin-openapi/openapi3"
)

const (
	defaultHTTPTimeout  = 30 * time.Second
	maxResponseBodySize = 32 << 20 // 32 MiB
)

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

// Client executes authenticated, validated requests against WSO2 Identity Server.
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
	tokens     *tokenCache
}

// New constructs a Client from cfg, configuring TLS trust from the system pool and optional CERT_PATH.
// TLS verification is never disabled.
func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("client config is required")
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if cfg.CertPath != "" {
		cleanPath := filepath.Clean(cfg.CertPath)
		pemBytes, err := os.ReadFile(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("read CERT_PATH %q: %w", cleanPath, err)
		}
		if !rootCAs.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("no valid PEM certificates found in %q", cleanPath)
		}
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    rootCAs,
			MinVersion: tls.VersionTLS12,
		},
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   defaultHTTPTimeout,
		},
		tokens: newTokenCache(),
	}, nil
}

// SetHTTPClient overrides the underlying HTTP client, primarily for testing.
func (c *Client) SetHTTPClient(hc *http.Client) {
	if hc != nil {
		c.httpClient = hc
	}
}

func (c *Client) setNow(fn func() time.Time) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	c.tokens.now = fn
}

// Do validates inputs against op, authenticates, executes the IS request, and decodes the JSON response.
func (c *Client) Do(ctx context.Context, op *catalog.Operation, pathParams map[string]string, query map[string]any, body any) (any, error) {
	if op == nil {
		return nil, errors.New("operation is required")
	}

	if err := ValidateInput(op, pathParams, query, body); err != nil {
		return nil, err
	}

	resolvedPath := resolvePathParams(op.Path, pathParams)
	fullURL, err := buildRequestURL(c.cfg.TenantBaseURL(), resolvedPath, query)
	if err != nil {
		return nil, err
	}

	bodyBytes, contentType, err := encodeRequestBody(op.Body, body)
	if err != nil {
		return nil, err
	}

	token, err := c.resolveToken(ctx, op.Scopes)
	if err != nil {
		return nil, err
	}

	return c.execute(ctx, op.Method, fullURL, contentType, bodyBytes, token, op)
}

// DoRaw executes an authenticated HTTP request against fullPath (relative to TenantBaseURL) without catalog validation.
func (c *Client) DoRaw(ctx context.Context, method, fullPath, contentType string, bodyBytes []byte) (any, error) {
	fullURL := strings.TrimRight(c.cfg.TenantBaseURL(), "/") + ensureLeadingSlash(fullPath)
	token, err := c.resolveToken(ctx, nil)
	if err != nil {
		return nil, err
	}
	return c.execute(ctx, method, fullURL, contentType, bodyBytes, token, nil)
}

func (c *Client) execute(ctx context.Context, method, fullURL, contentType string, bodyBytes []byte, token string, op *catalog.Operation) (any, error) {
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	// Security headers are always owned by the client and never taken from caller input.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json, application/scim+json")
	if contentType != "" && len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read HTTP response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newAPIError(resp.StatusCode, respBytes, op)
	}

	if resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(respBytes)) == 0 {
		return nil, nil
	}

	var parsed any
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		respCT := strings.ToLower(resp.Header.Get("Content-Type"))
		if strings.Contains(respCT, "json") {
			return nil, fmt.Errorf("decode JSON response: %w", err)
		}
		return string(respBytes), nil
	}
	return parsed, nil
}

func ensureLeadingSlash(p string) string {
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

func resolvePathParams(pathTemplate string, pathParams map[string]string) string {
	return pathParamPattern.ReplaceAllStringFunc(pathTemplate, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		if val, ok := pathParams[name]; ok {
			return url.PathEscape(val)
		}
		return match
	})
}

func buildRequestURL(baseURL, resolvedPath string, query map[string]any) (string, error) {
	rawURL := strings.TrimRight(baseURL, "/") + ensureLeadingSlash(resolvedPath)
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse request URL: %w", err)
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			if v == nil {
				continue
			}
			switch items := v.(type) {
			case []string:
				for _, s := range items {
					q.Add(k, s)
				}
			case []any:
				for _, item := range items {
					q.Add(k, formatScalar(item))
				}
			default:
				q.Set(k, formatScalar(v))
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func formatScalar(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprint(val)
	}
}

func encodeRequestBody(spec *catalog.BodySpec, body any) ([]byte, string, error) {
	if spec == nil || body == nil {
		return nil, "", nil
	}

	ct := spec.ContentType
	if ct == "" {
		ct = "application/json"
	}

	switch {
	case ct == "application/x-www-form-urlencoded":
		return encodeFormBody(body)
	case ct == "multipart/form-data":
		return encodeMultipartBody(spec.Schema, body)
	case ct == "application/json" || strings.HasSuffix(ct, "+json"):
		data, err := encodeJSONBody(spec.Schema, body)
		if err != nil {
			return nil, "", err
		}
		return data, ct, nil
	default:
		switch val := body.(type) {
		case []byte:
			return val, ct, nil
		case string:
			return []byte(val), ct, nil
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, "", fmt.Errorf("encode request body: %w", err)
			}
			return data, ct, nil
		}
	}
}

func encodeJSONBody(schema *openapi3.SchemaRef, body any) ([]byte, error) {
	switch val := body.(type) {
	case []byte:
		return val, nil
	case string:
		isStringSchema := schema != nil && schema.Value != nil && schema.Value.Type != nil && schema.Value.Type.Is(openapi3.TypeString)
		if !isStringSchema && json.Valid([]byte(val)) {
			return []byte(val), nil
		}
		data, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("marshal JSON string body: %w", err)
		}
		return data, nil
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal JSON body: %w", err)
		}
		return data, nil
	}
}

func encodeFormBody(body any) ([]byte, string, error) {
	switch val := body.(type) {
	case string:
		return []byte(val), "application/x-www-form-urlencoded", nil
	case url.Values:
		return []byte(val.Encode()), "application/x-www-form-urlencoded", nil
	case map[string]string:
		v := url.Values{}
		for k, s := range val {
			v.Set(k, s)
		}
		return []byte(v.Encode()), "application/x-www-form-urlencoded", nil
	case map[string]any:
		v := url.Values{}
		for k, item := range val {
			if item == nil {
				continue
			}
			v.Set(k, formatScalar(item))
		}
		return []byte(v.Encode()), "application/x-www-form-urlencoded", nil
	default:
		return nil, "", fmt.Errorf("unsupported form-urlencoded body type %T", body)
	}
}

func encodeMultipartBody(schema *openapi3.SchemaRef, body any) ([]byte, string, error) {
	fields, err := toStringMap(body)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		val := fields[k]
		if val == nil {
			continue
		}
		if isBinaryField(schema, k) {
			s, ok := val.(string)
			if !ok {
				return nil, "", fmt.Errorf("multipart field %q must be a base64 string", k)
			}
			raw, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return nil, "", fmt.Errorf("invalid base64 for multipart field %q: %w", k, err)
			}
			fw, err := mw.CreateFormFile(k, k)
			if err != nil {
				return nil, "", fmt.Errorf("create multipart file field %q: %w", k, err)
			}
			if _, err := fw.Write(raw); err != nil {
				return nil, "", fmt.Errorf("write multipart file field %q: %w", k, err)
			}
			continue
		}
		if err := mw.WriteField(k, formatScalar(val)); err != nil {
			return nil, "", fmt.Errorf("write multipart field %q: %w", k, err)
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func toStringMap(body any) (map[string]any, error) {
	if m, ok := body.(map[string]any); ok {
		return m, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("expected object body: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("expected object body: %w", err)
	}
	return m, nil
}

func isBinaryField(schema *openapi3.SchemaRef, name string) bool {
	if schema == nil || schema.Value == nil || schema.Value.Properties == nil {
		return false
	}
	prop, ok := schema.Value.Properties[name]
	if !ok || prop == nil || prop.Value == nil {
		return false
	}
	return strings.EqualFold(prop.Value.Format, "binary")
}
