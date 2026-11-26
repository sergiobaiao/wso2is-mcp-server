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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSyntheticVersion999(t *testing.T) {
	versionsFS := os.DirFS(filepath.Join("testdata", "versions"))
	cat, err := Load(versionsFS, "9.9.9")
	if err != nil {
		t.Fatalf("Load(9.9.9) failed: %v", err)
	}
	if cat.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9", cat.Version)
	}

	op1, ok := cat.Get("governance_security.listAllowedCORSOrigins")
	if !ok {
		t.Fatalf("missing governance_security.listAllowedCORSOrigins")
	}
	if op1.Kind != KindRead || op1.Area != "governance_security" {
		t.Errorf("unexpected op1: %+v", op1)
	}

	op2, ok := cat.Get("applications.org.createApplication")
	if !ok {
		t.Fatalf("missing applications.org.createApplication")
	}
	if op2.Kind != KindWrite || !op2.OrgScoped() {
		t.Errorf("unexpected op2: %+v", op2)
	}
}

func TestBuildPerOperationToolDefsForSyntheticVersion(t *testing.T) {
	versionsFS := os.DirFS(filepath.Join("testdata", "versions"))
	cat, err := Load(versionsFS, "9.9.9")
	if err != nil {
		t.Fatalf("Load(9.9.9) failed: %v", err)
	}

	type toolDef struct {
		Name string
		Kind Kind
		Path string
	}
	var defs []toolDef
	for _, op := range cat.Operations {
		defs = append(defs, toolDef{
			Name: op.Area + "_" + op.OperationID,
			Kind: op.Kind,
			Path: op.Path,
		})
	}
	if len(defs) != 5 {
		t.Fatalf("expected 5 per-operation defs for synthetic version (2 spec + 3 extras), got %d", len(defs))
	}
}

func TestUnknownVersionListsAvailable(t *testing.T) {
	versionsFS := os.DirFS(filepath.Join("testdata", "versions"))
	_, err := Load(versionsFS, "0.0.0")
	if err == nil {
		t.Fatal("expected error for unknown IS_VERSION")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("error %q should list available version 9.9.9", err.Error())
	}
}
