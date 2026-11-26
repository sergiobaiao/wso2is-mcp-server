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

// Package branding implements the curated branding_notifications tools
// (contracts/admin-curated-tools.md).
package branding

import (
	"context"
	"errors"
	"net/http"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultEmailSender = "EmailPublisher"
	defaultSMSSender   = "SMSPublisher"
)

// Tools returns the five branding and notification tools.
func Tools(cat *catalog.Catalog, cl *client.Client, cfg *config.Config) ([]toolkit.ToolDef, error) {
	ids := [][2]string{
		{"branding_notifications.getBrandingPreference", ""},
		{"branding_notifications.updateBrandingPreference", "branding_notifications.org.updateBrandingPreference"},
		{"branding_notifications.getEmailSender", "branding_notifications.org.getEmailSender"},
		{"branding_notifications.createEmailSender", "branding_notifications.org.createEmailSender"},
		{"branding_notifications.updateEmailSender", "branding_notifications.org.updateEmailSender"},
		{"branding_notifications.getSMSSender", "branding_notifications.org.getSMSSender"},
		// The v2 spec reuses createSMSSender for push senders; the SMS create is postNotificationSendersSms.
		{"branding_notifications.postNotificationSendersSms", "branding_notifications.org.postNotificationSendersSms"},
		{"branding_notifications.updateSMSSender", "branding_notifications.org.updateSMSSender"},
		{"branding_notifications.updateOrgEmailTemplate", "branding_notifications.org.updateOrgEmailTemplate"},
	}
	ops := make([]toolkit.OpPair, len(ids))
	for i, v := range ids {
		var err error
		if ops[i], err = toolkit.LookupPair(cat, v[0], v[1]); err != nil {
			return nil, err
		}
	}
	getB, updB, getEmail, addEmail, updEmail, getSMS, addSMS, updSMS, updTemplate :=
		ops[0], ops[1], ops[2], ops[3], ops[4], ops[5], ops[6], ops[7], ops[8]
	if cfg == nil {
		cfg = &config.Config{}
	}
	tenant := cfg.TenantDomain
	if tenant == "" {
		tenant = config.DefaultTenantDomain
	}

	brandingOpts := []mcp.ToolOption{
		mcp.WithString("type", mcp.Enum("ORG", "APP", "CUSTOM"), mcp.Description("Branding level; default ORG.")),
		mcp.WithString("name", mcp.Description("Tenant domain for ORG (default), application id for APP.")),
		mcp.WithString("locale", mcp.Description("Locale, e.g. en-US.")),
	}
	brandingTarget := func(args map[string]any) (map[string]any, error) {
		t := map[string]any{"type": "ORG", "name": tenant}
		for _, k := range []string{"type", "name", "locale"} {
			v, err := toolkit.OptString(args, k)
			if err != nil {
				return nil, err
			}
			if v != "" {
				t[k] = v
			}
		}
		return t, nil
	}
	senderOpts := func(kind string, fields []mcp.ToolOption) []mcp.ToolOption {
		return append([]mcp.ToolOption{
			mcp.WithString("name", mcp.Description("Sender name; default "+kind+".")),
			mcp.WithArray("properties", mcp.Description("Extra provider properties as {key, value} objects. Secrets are sent to IS and never returned.")),
		}, fields...)
	}

	return []toolkit.ToolDef{
		{
			Name: "get_branding", Layer: toolkit.LayerCurated, Kind: catalog.KindRead,
			Scopes: getB.Scopes(), Operation: getB.Root,
			Description: "Get the branding preference (theme, logos, colors, texts) of the login pages of WSO2 Identity Server.",
			Options:     brandingOpts,
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				q, err := brandingTarget(args)
				if err != nil {
					return nil, err
				}
				return toolkit.Call(ctx, cl, getB, nil, q, nil)
			},
		},
		{
			Name: "update_branding", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: updB.Scopes(), OrgCapable: true, Operation: updB.Root,
			Description: "Replace the branding preference of the login pages of WSO2 Identity Server. Read it with get_branding first and send the whole preference object.",
			Options: append(brandingOpts,
				mcp.WithObject("preference", mcp.Required(), mcp.Description("The full branding preference object (theme, layout, organizationDetails, images, urls, configs)."))),
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body, err := brandingTarget(args)
				if err != nil {
					return nil, err
				}
				pref, err := toolkit.OptObject(args, "preference")
				if err != nil {
					return nil, err
				}
				if pref == nil {
					return nil, errors.New("invalid input: preference: is required")
				}
				body["preference"] = pref
				return toolkit.Call(ctx, cl, updB, nil, nil, body)
			},
		},
		{
			Name: "configure_email_sender", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: toolkit.UnionScopes(getEmail.Root, addEmail.Root, updEmail.Root), OrgCapable: true, Operation: addEmail.Root,
			Description: "Create or update the SMTP email sender WSO2 Identity Server uses for notifications.",
			Options: senderOpts(defaultEmailSender, []mcp.ToolOption{
				mcp.WithString("smtp_server_host", mcp.Required(), mcp.Description("SMTP server host.")),
				mcp.WithNumber("smtp_port", mcp.Required(), mcp.Description("SMTP port, e.g. 587.")),
				mcp.WithString("from_address", mcp.Required(), mcp.Description("Sender email address.")),
				mcp.WithString("auth_type", mcp.Description("Authentication type, e.g. BASIC or CLIENT_CREDENTIAL; credentials go in properties.")),
			}),
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				body := map[string]any{}
				for _, f := range [][2]string{{"smtp_server_host", "smtpServerHost"}, {"from_address", "fromAddress"}} {
					v, err := toolkit.RequireString(args, f[0])
					if err != nil {
						return nil, err
					}
					body[f[1]] = v
				}
				port, ok, err := toolkit.OptInt(args, "smtp_port")
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, errors.New("invalid input: smtp_port: is required")
				}
				body["smtpPort"] = port
				toolkit.PutIf(body, args, map[string]string{"auth_type": "authType", "properties": "properties"})
				return upsert(ctx, cl, args, defaultEmailSender, getEmail, addEmail, updEmail, body)
			},
		},
		{
			Name: "configure_sms_sender", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: toolkit.UnionScopes(getSMS.Root, addSMS.Root, updSMS.Root), OrgCapable: true, Operation: addSMS.Root,
			Description: "Create or update the SMS sender (provider such as Twilio or Vonage) WSO2 Identity Server uses for OTPs and notifications.",
			Options: senderOpts(defaultSMSSender, []mcp.ToolOption{
				mcp.WithString("provider", mcp.Required(), mcp.Description("SMS provider, e.g. Twilio, Vonage or Custom.")),
				mcp.WithString("content_type", mcp.Enum("JSON", "FORM"), mcp.Description("Payload content type; default JSON.")),
				mcp.WithString("provider_url", mcp.Description("Provider API URL.")),
				mcp.WithString("key", mcp.Description("Provider key or account id.")),
				mcp.WithString("secret", mcp.Description("Provider secret. Sent to IS and never returned.")),
				mcp.WithString("sender", mcp.Description("Sender number or name.")),
			}),
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				provider, err := toolkit.RequireString(args, "provider")
				if err != nil {
					return nil, err
				}
				body := map[string]any{"provider": provider, "contentType": "JSON"}
				toolkit.PutIf(body, args, map[string]string{
					"content_type": "contentType", "provider_url": "providerURL", "key": "key",
					"secret": "secret", "sender": "sender", "properties": "properties",
				})
				return upsert(ctx, cl, args, defaultSMSSender, getSMS, addSMS, updSMS, body)
			},
		},
		{
			Name: "update_email_template", Layer: toolkit.LayerCurated, Kind: catalog.KindWrite,
			Scopes: updTemplate.Scopes(), OrgCapable: true, Operation: updTemplate.Root,
			Description: "Update the organization email template of a template type and locale in WSO2 Identity Server (email templates v2).",
			Options: []mcp.ToolOption{
				mcp.WithString("template_type_id", mcp.Required(), mcp.Description("Template type id (base64url of the type name, e.g. of AccountConfirmation).")),
				mcp.WithString("locale", mcp.Required(), mcp.Description("Locale, e.g. en_US.")),
				mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject.")),
				mcp.WithString("body", mcp.Required(), mcp.Description("Email body.")),
				mcp.WithString("footer", mcp.Description("Email footer.")),
				mcp.WithString("content_type", mcp.Description("Content type; default text/html.")),
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				path := map[string]string{}
				for _, f := range [][2]string{{"template_type_id", "template-type-id"}, {"locale", "locale"}} {
					v, err := toolkit.RequireString(args, f[0])
					if err != nil {
						return nil, err
					}
					path[f[1]] = v
				}
				body := map[string]any{"contentType": "text/html"}
				for _, f := range []string{"subject", "body"} {
					v, err := toolkit.RequireString(args, f)
					if err != nil {
						return nil, err
					}
					body[f] = v
				}
				toolkit.PutIf(body, args, map[string]string{"footer": "footer", "content_type": "contentType"})
				// EmailTemplateWithID is allOf an object and an array in the 7.3 spec, which no body
				// satisfies, so the body is validated by IS only.
				return toolkit.CallUnvalidatedBody(ctx, cl, updTemplate, path, nil, body)
			},
		},
	}, nil
}

// upsert updates the named sender when it exists and creates it when IS answers 404.
func upsert(ctx context.Context, cl *client.Client, args map[string]any, defaultName string,
	get, create, update toolkit.OpPair, body map[string]any) (any, error) {
	name, err := toolkit.OptString(args, "name")
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = defaultName
	}
	path := map[string]string{"sender-name": name}
	_, err = toolkit.Call(ctx, cl, get, path, nil, nil)
	var apiErr *client.APIError
	// The v2 sender schemas are an ambiguous oneOf (SMTP and HTTP variants both match), so the
	// body is validated by IS only.
	switch {
	case err == nil:
		return toolkit.CallUnvalidatedBody(ctx, cl, update, path, nil, body)
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		body["name"] = name
		return toolkit.CallUnvalidatedBody(ctx, cl, create, nil, nil, body)
	default:
		return nil, err
	}
}
