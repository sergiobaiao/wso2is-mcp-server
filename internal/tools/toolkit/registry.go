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

package toolkit

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/mark3labs/mcp-go/server"
)

type registryEntry struct {
	def        ToolDef
	serverTool server.ServerTool
}

// Registry validates, deduplicates, filters, and stores MCP tools before attaching them to a server.
type Registry struct {
	mu               sync.RWMutex
	readOnly         bool
	maxResponseBytes int
	entries          []registryEntry
	seenNames        map[string]bool
}

// NewRegistry creates an empty Registry configured for read-only filtering and response truncation.
func NewRegistry(readOnly bool, maxResponseBytes int) *Registry {
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}
	return &Registry{
		readOnly:         readOnly,
		maxResponseBytes: maxResponseBytes,
		seenNames:        make(map[string]bool),
	}
}

// Register validates def, rejects duplicate names, skips non-read tools when readOnly is true,
// and builds the underlying server.ServerTool.
func (r *Registry) Register(def ToolDef) error {
	if err := ValidateName(def.Name); err != nil {
		return err
	}
	if err := validateKind(def.Kind); err != nil {
		return fmt.Errorf("tool %q: %w", def.Name, err)
	}
	if def.Handler == nil {
		return fmt.Errorf("tool %q: handler is required", def.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.seenNames[def.Name] {
		return fmt.Errorf("duplicate tool name %q", def.Name)
	}

	if r.readOnly && def.Kind != catalog.KindRead {
		r.seenNames[def.Name] = true
		return nil
	}

	st, err := BuildTool(def, r.maxResponseBytes)
	if err != nil {
		return err
	}

	r.seenNames[def.Name] = true
	r.entries = append(r.entries, registryEntry{
		def:        def,
		serverTool: st,
	})
	return nil
}

// MustRegister registers all defs and panics if any definition fails validation or collides.
func (r *Registry) MustRegister(defs ...ToolDef) {
	for _, def := range defs {
		if err := r.Register(def); err != nil {
			panic(err)
		}
	}
}

// ServerTools returns a copy of all registered server.ServerTool entries in registration order.
func (r *Registry) ServerTools() []server.ServerTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]server.ServerTool, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.serverTool
	}
	return out
}

// Count returns the number of registered tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Names returns the registered tool names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.def.Name
	}
	return out
}

// Scopes returns the sorted unique union of OAuth scopes declared by registered tools.
func (r *Registry) Scopes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	set := make(map[string]bool)
	for _, e := range r.entries {
		for _, s := range e.def.Scopes {
			s = strings.TrimSpace(s)
			if s != "" {
				set[s] = true
			}
		}
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// AddToServer registers all tools held by the registry on s.
func (r *Registry) AddToServer(s *server.MCPServer) {
	if s == nil {
		return
	}
	tools := r.ServerTools()
	if len(tools) > 0 {
		s.AddTools(tools...)
	}
}
