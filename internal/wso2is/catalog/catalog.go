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
	"fmt"
	"sort"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/getkin/kin-openapi/openapi3"
)

// Availability describes whether an operation can be invoked under the active runtime Config.
type Availability string

const (
	AvailabilityAvailable        Availability = "available"
	AvailabilityFlowDisabled     Availability = "flow_disabled"
	AvailabilityDeprecatedHidden Availability = "deprecated_hidden"
	AvailabilityReadOnlyHidden   Availability = "read_only_hidden"
)

// SourceInfo holds provenance from the vendored SOURCE file.
type SourceInfo struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	RootPath   string `json:"root_path"`
	OrgPath    string `json:"org_path"`
}

// Stats summarizes the loaded catalog for golden-file regression testing.
type Stats struct {
	Version          string         `json:"version"`
	SpecFiles        int            `json:"spec_files"`
	RootSpecFiles    int            `json:"root_spec_files"`
	OrgSpecFiles     int            `json:"org_spec_files"`
	PublishedOps     int            `json:"published_ops"`
	DuplicateSkipped int            `json:"duplicate_skipped"`
	ExtraOps         int            `json:"extra_ops"`
	UniqueOps        int            `json:"unique_ops"`
	ByFamily         map[string]int `json:"by_family"`
	ByArea           map[string]int `json:"by_area"`
	ByKind           map[string]int `json:"by_kind"`
	FlowOps          int            `json:"flow_ops"`
	DeprecatedOps    int            `json:"deprecated_ops"`
}

// Catalog holds all normalized IS operations for one version.
type Catalog struct {
	Version    string
	Source     SourceInfo
	Operations []*Operation
	byID       map[string]*Operation
	stats      Stats
}

func buildCatalog(version string, source SourceInfo, specFiles []*SpecFile) (*Catalog, error) {
	stats := Stats{
		Version:   version,
		SpecFiles: len(specFiles),
		ByFamily:  make(map[string]int),
		ByArea:    make(map[string]int),
		ByKind:    make(map[string]int),
	}

	seenRoutes := make(map[string]bool)
	seenIDs := make(map[string]bool)
	var ops []*Operation

	for _, sf := range specFiles {
		if sf.Family == FamilyRoot {
			stats.RootSpecFiles++
		} else {
			stats.OrgSpecFiles++
		}

		if sf.Doc == nil || sf.Doc.Paths == nil {
			continue
		}

		seenInFile := make(map[string]bool)
		pathKeys := sf.Doc.Paths.InMatchingOrder()
		sort.Strings(pathKeys)

		for _, rawPath := range pathKeys {
			pi := sf.Doc.Paths.Find(rawPath)
			if pi == nil {
				continue
			}
			fullPath := joinPath(sf.PathPrefix, rawPath)
			for _, pair := range orderedMethods(pi) {
				method, oaOp := pair.method, pair.op
				stats.PublishedOps++

				routeKey := fmt.Sprintf("%s|%s|%s", sf.Family, method, fullPath)
				if seenRoutes[routeKey] {
					stats.DuplicateSkipped++
					continue
				}
				seenRoutes[routeKey] = true

				opID := sanitizeOperationID(oaOp.OperationID)
				if opID == "" || seenInFile[opID] {
					opID = deriveOperationID(method, rawPath)
				}
				seenInFile[opID] = true

				fullID := makeOperationID(sf.Area, sf.Family, opID)
				if seenIDs[fullID] {
					opID = deriveOperationID(method, fullPath)
					fullID = makeOperationID(sf.Area, sf.Family, opID)
					if seenIDs[fullID] {
						base := opID
						for n := 2; seenIDs[fullID]; n++ {
							opID = fmt.Sprintf("%s_%d", base, n)
							fullID = makeOperationID(sf.Area, sf.Family, opID)
						}
					}
				}
				seenIDs[fullID] = true

				desc := cleanText(oaOp.Description)
				summary := buildSummary(oaOp.Summary, desc)
				kind := classifyKind(method, fullPath)
				flow := isFlowOperation(sf, method, fullPath)
				deprecated := sf.Deprecated || oaOp.Deprecated

				var tags []string
				if len(oaOp.Tags) > 0 {
					tags = append([]string(nil), oaOp.Tags...)
				}

				op := &Operation{
					ID:          fullID,
					OperationID: opID,
					Area:        sf.Area,
					Family:      sf.Family,
					Method:      method,
					Path:        fullPath,
					PathPrefix:  sf.PathPrefix,
					Summary:     summary,
					Description: desc,
					Kind:        kind,
					Scopes:      extractScopes(oaOp),
					Tags:        tags,
					Params:      extractParams(pi.Parameters, oaOp.Parameters),
					Body:        extractBody(oaOp.RequestBody),
					Deprecated:  deprecated,
					Flow:        flow,
					Source:      fmt.Sprintf("%s/%s#%s %s", sf.Family, sf.FileName, method, rawPath),
				}
				ops = append(ops, op)
			}
		}
	}

	for _, extra := range extraOperations() {
		if seenIDs[extra.ID] {
			return nil, fmt.Errorf("duplicate extra operation id %q", extra.ID)
		}
		seenIDs[extra.ID] = true
		ops = append(ops, extra)
		stats.ExtraOps++
	}

	sort.Slice(ops, func(i, j int) bool { return ops[i].ID < ops[j].ID })

	byID := make(map[string]*Operation, len(ops))
	for _, op := range ops {
		byID[op.ID] = op
		stats.ByFamily[string(op.Family)]++
		stats.ByArea[op.Area]++
		stats.ByKind[string(op.Kind)]++
		if op.Flow {
			stats.FlowOps++
		}
		if op.Deprecated {
			stats.DeprecatedOps++
		}
	}
	stats.UniqueOps = len(ops)

	return &Catalog{
		Version:    version,
		Source:     source,
		Operations: ops,
		byID:       byID,
		stats:      stats,
	}, nil
}

type methodOp struct {
	method string
	op     *openapi3.Operation
}

func orderedMethods(pi *openapi3.PathItem) []methodOp {
	candidates := []methodOp{
		{"GET", pi.Get},
		{"POST", pi.Post},
		{"PUT", pi.Put},
		{"PATCH", pi.Patch},
		{"DELETE", pi.Delete},
		{"HEAD", pi.Head},
	}
	var out []methodOp
	for _, c := range candidates {
		if c.op != nil {
			out = append(out, c)
		}
	}
	return out
}

// Get returns an operation by its canonical ID.
func (c *Catalog) Get(id string) (*Operation, bool) {
	op, ok := c.byID[id]
	return op, ok
}

// Areas returns all area names present in the catalog, sorted alphabetically.
func (c *Catalog) Areas() []string {
	set := make(map[string]bool)
	for _, op := range c.Operations {
		set[op.Area] = true
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Stats returns summary counts for the loaded catalog.
func (c *Catalog) Stats() Stats {
	return c.stats
}

// Availability returns whether op can be invoked under cfg.
func (c *Catalog) Availability(op *Operation, cfg *config.Config) Availability {
	if op == nil {
		return AvailabilityDeprecatedHidden
	}
	if cfg != nil {
		if op.Flow && !cfg.AllowFlowAPIs {
			return AvailabilityFlowDisabled
		}
		if op.Deprecated && !cfg.IncludeDeprecated {
			return AvailabilityDeprecatedHidden
		}
		if cfg.ReadOnly && op.Kind != KindRead {
			return AvailabilityReadOnlyHidden
		}
	} else {
		if op.Flow {
			return AvailabilityFlowDisabled
		}
		if op.Deprecated {
			return AvailabilityDeprecatedHidden
		}
	}
	return AvailabilityAvailable
}

// SearchFilter narrows catalog searches.
type SearchFilter struct {
	Area         string
	Kind         Kind
	Organization *bool
	Limit        int
}

// HasPathPrefix checks whether a path belongs to an area/prefix.
func (o *Operation) HasPathParam(name string) bool {
	for _, p := range o.Params {
		if p.In == "path" && p.Name == name {
			return true
		}
	}
	return false
}

// RefusalReason returns the human-readable refusal message when an operation is unavailable.
func RefusalReason(op *Operation, avail Availability) string {
	switch avail {
	case AvailabilityFlowDisabled:
		return fmt.Sprintf("operation %s is a flow API and is disabled; set ALLOW_FLOW_APIS=true to enable it", op.ID)
	case AvailabilityDeprecatedHidden:
		return fmt.Sprintf("operation %s is deprecated; set INCLUDE_DEPRECATED=true", op.ID)
	case AvailabilityReadOnlyHidden:
		return fmt.Sprintf("operation %s changes data and READ_ONLY=true is enabled", op.ID)
	default:
		if strings.TrimSpace(op.ID) == "" {
			return "operation is not available"
		}
		return ""
	}
}
