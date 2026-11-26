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
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// Load reads and normalizes all OpenAPI specs for the given IS version from fsys.
func Load(fsys fs.FS, version string) (*Catalog, error) {
	if fsys == nil {
		return nil, fmt.Errorf("spec filesystem is nil")
	}
	baseDir, err := resolveVersionDir(fsys, version)
	if err != nil {
		return nil, err
	}

	source, err := loadSourceInfo(fsys, path.Join(baseDir, "SOURCE"))
	if err != nil {
		return nil, err
	}

	var specFiles []*SpecFile
	for _, fam := range []Family{FamilyRoot, FamilyOrg} {
		dir := path.Join(baseDir, string(fam))
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return nil, fmt.Errorf("read %s specs directory %q: %w", fam, dir, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fname := entry.Name()
			if !strings.HasSuffix(fname, ".yaml") && !strings.HasSuffix(fname, ".yml") {
				continue
			}
			raw, err := fs.ReadFile(fsys, path.Join(dir, fname))
			if err != nil {
				return nil, fmt.Errorf("read spec %s/%s: %w", fam, fname, err)
			}
			sf, err := parseSpecFile(fam, fname, raw)
			if err != nil {
				return nil, fmt.Errorf("parse spec %s/%s: %w", fam, fname, err)
			}
			specFiles = append(specFiles, sf)
		}
	}

	return buildCatalog(version, source, specFiles)
}

func resolveVersionDir(fsys fs.FS, version string) (string, error) {
	candidates := []string{
		path.Join("wso2is", version),
		version,
	}
	for _, c := range candidates {
		if info, err := fs.Stat(fsys, c); err == nil && info.IsDir() {
			return c, nil
		}
	}

	// Collect available versions for a helpful error message.
	var available []string
	for _, parent := range []string{"wso2is", "."} {
		entries, err := fs.ReadDir(fsys, parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && e.Name() != "wso2is" {
				available = append(available, e.Name())
			}
		}
		if len(available) > 0 {
			break
		}
	}
	sort.Strings(available)
	return "", fmt.Errorf("unknown IS_VERSION %q; available versions: %s", version, strings.Join(available, ", "))
}

func loadSourceInfo(fsys fs.FS, sourcePath string) (SourceInfo, error) {
	raw, err := fs.ReadFile(fsys, sourcePath)
	if err != nil {
		return SourceInfo{}, fmt.Errorf("read %s: %w", sourcePath, err)
	}
	var info SourceInfo
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "repository":
			info.Repository = v
		case "commit":
			info.Commit = v
		case "root_path":
			info.RootPath = v
		case "org_path":
			info.OrgPath = v
		}
	}
	return info, nil
}

func parseSpecFile(family Family, fileName string, raw []byte) (*SpecFile, error) {
	baseName := strings.TrimSuffix(strings.TrimSuffix(fileName, ".yaml"), ".yml")
	area, ok := fileToArea[baseName]
	if !ok {
		return nil, fmt.Errorf("unmapped spec file %q in area table", baseName)
	}

	var probe struct {
		Swagger  string `yaml:"swagger"`
		BasePath string `yaml:"basePath"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("probe YAML: %w", err)
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	var doc3 *openapi3.T
	if strings.HasPrefix(probe.Swagger, "2.") {
		var generic any
		if err := yaml.Unmarshal(raw, &generic); err != nil {
			return nil, fmt.Errorf("unmarshal swagger 2.0 YAML: %w", err)
		}
		jsonData, err := json.Marshal(toJSONCompatible(generic))
		if err != nil {
			return nil, fmt.Errorf("convert swagger 2.0 to JSON: %w", err)
		}
		var doc2 openapi2.T
		if err := json.Unmarshal(jsonData, &doc2); err != nil {
			return nil, fmt.Errorf("parse swagger 2.0: %w", err)
		}
		converted, err := openapi2conv.ToV3WithLoader(&doc2, loader, nil)
		if err != nil {
			return nil, fmt.Errorf("convert swagger 2.0 to v3: %w", err)
		}
		doc3 = converted
	} else {
		normalizedRaw, err := normalizeOpenAPI3Raw(raw)
		if err != nil {
			return nil, fmt.Errorf("normalize OpenAPI 3 refs: %w", err)
		}
		loaded, err := loader.LoadFromData(normalizedRaw)
		if err != nil {
			return nil, fmt.Errorf("load OpenAPI 3: %w", err)
		}
		doc3 = loaded
	}

	servers := make([]string, 0, len(doc3.Servers))
	for _, s := range doc3.Servers {
		servers = append(servers, s.URL)
	}

	return &SpecFile{
		Family:     family,
		Name:       baseName,
		FileName:   fileName,
		Area:       area,
		PathPrefix: resolvePathPrefix(family, baseName, servers, probe.BasePath),
		Deprecated: deprecatedSpecs[baseName],
		Flow:       flowSpecs[baseName],
		Doc:        doc3,
	}, nil
}

// toJSONCompatible recursively converts map[any]any (e.g. YAML integer status keys) to map[string]any.
func toJSONCompatible(v any) any {
	switch val := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[fmt.Sprint(k)] = toJSONCompatible(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = toJSONCompatible(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = toJSONCompatible(item)
		}
		return out
	default:
		return v
	}
}

// normalizeOpenAPI3Raw repairs known OpenAPI 3 spec defects in normalization code (Constitution Principle II):
// 1. Rewrites leftover Swagger 2.0 #/definitions/ pointers to #/components/schemas/.
// 2. Synthesizes missing components referenced in claim-management.yaml (FileUpload, fileTypeHeaderParam, preserveClaimsParam).
func normalizeOpenAPI3Raw(raw []byte) ([]byte, error) {
	text := strings.ReplaceAll(string(raw), "#/definitions/", "#/components/schemas/")
	if !strings.Contains(text, "#/components/schemas/FileUpload") &&
		!strings.Contains(text, "#/components/parameters/fileTypeHeaderParam") &&
		!strings.Contains(text, "#/components/parameters/preserveClaimsParam") {
		return []byte(text), nil
	}

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	comps, _ := doc["components"].(map[string]any)
	if comps == nil {
		comps = make(map[string]any)
		doc["components"] = comps
	}
	if strings.Contains(text, "#/components/schemas/FileUpload") {
		schemas, _ := comps["schemas"].(map[string]any)
		if schemas == nil {
			schemas = make(map[string]any)
			comps["schemas"] = schemas
		}
		if _, ok := schemas["FileUpload"]; !ok {
			schemas["FileUpload"] = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file": map[string]any{"type": "string", "format": "binary"},
				},
			}
		}
	}
	params, _ := comps["parameters"].(map[string]any)
	if params == nil {
		params = make(map[string]any)
		comps["parameters"] = params
	}
	if strings.Contains(text, "#/components/parameters/fileTypeHeaderParam") {
		if _, ok := params["fileTypeHeaderParam"]; !ok {
			params["fileTypeHeaderParam"] = map[string]any{
				"name":   "Accept",
				"in":     "header",
				"schema": map[string]any{"type": "string"},
			}
		}
	}
	if strings.Contains(text, "#/components/parameters/preserveClaimsParam") {
		if _, ok := params["preserveClaimsParam"]; !ok {
			params["preserveClaimsParam"] = map[string]any{
				"name":   "preserveClaims",
				"in":     "query",
				"schema": map[string]any{"type": "boolean"},
			}
		}
	}
	return yaml.Marshal(doc)
}
