/*
 * Copyright (c) 2025, WSO2 LLC. (http://www.wso2.com).
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

package asgardeo

import (
	"context"
	"sync"
	"time"

	"github.com/asgardeo/go/pkg/config"
	"github.com/asgardeo/go/pkg/sdk"
	internal_config "github.com/asgardeo/mcp/internal/config"
)

var (
	clientInstance *sdk.Client
	once           sync.Once
	initErr        error
)

// NewClient initializes an Asgardeo management client with client credentials.
func NewClient(ctx context.Context, baseURL, clientID, clientSecret string, certPath *string) (*sdk.Client, error) {
	cfg := config.DefaultClientConfig().
		WithBaseURL(baseURL).
		WithTimeout(10*time.Second).
		WithClientCredentials(clientID, clientSecret).
		WithCertificatePath(certPath)

	return sdk.New(cfg)
}

// GetClientInstance returns the singleton Asgardeo client.
func GetClientInstance(ctx context.Context) (*sdk.Client, error) {
	once.Do(func() {
		cfg, err := internal_config.LoadFor(internal_config.BinaryAdmin)
		if err != nil {
			initErr = err
			return
		}
		var certPath *string
		if cfg.CertPath != "" {
			cp := cfg.CertPath
			certPath = &cp
		}

		clientInstance, initErr = NewClient(ctx, cfg.TenantBaseURL(), cfg.ClientID, cfg.ClientSecret, certPath)
	})

	return clientInstance, initErr
}
