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
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
)

func BenchmarkSearch(b *testing.B) {
	cat, err := Load(apispecs.FS, "7.3.0")
	if err != nil {
		b.Fatalf("Load(7.3.0): %v", err)
	}

	b.Run("multi_token_unfiltered", func(b *testing.B) {
		for b.Loop() {
			matches, total := cat.Search("cors origins", SearchFilter{})
			if total == 0 || len(matches) == 0 {
				b.Fatal("expected matches")
			}
		}
	})

	b.Run("exact_operation_id", func(b *testing.B) {
		for b.Loop() {
			matches, total := cat.Search("getCORSOrigins", SearchFilter{})
			if total == 0 || len(matches) == 0 {
				b.Fatal("expected matches")
			}
		}
	})

	b.Run("filtered_area_kind", func(b *testing.B) {
		org := false
		filter := SearchFilter{
			Area:         "applications",
			Kind:         KindRead,
			Organization: &org,
			Limit:        20,
		}
		for b.Loop() {
			matches, total := cat.Search("oidc inbound protocol", filter)
			if total == 0 || len(matches) == 0 {
				b.Fatal("expected matches")
			}
		}
	})
}
