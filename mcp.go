// SPDX-License-Identifier: MIT

// Model Context Protocol: initialize, tools/list and tools/call.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

var mcpTools = []any{
	map[string]any{"name": "create_repo", "description": "Create a repository", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}},
	map[string]any{"name": "get_repo", "description": "Get a repository", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}}, "required": []string{"owner", "repo"}}},
	map[string]any{"name": "create_issue", "description": "Create an issue", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}}, "required": []string{"owner", "repo", "title"}}},
	map[string]any{"name": "list_issues", "description": "List issues", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "state": map[string]any{"type": "string"}}, "required": []string{"owner", "repo"}}},
	map[string]any{"name": "create_pull_request", "description": "Create a pull request", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "head": map[string]any{"type": "string"}, "base": map[string]any{"type": "string"}}, "required": []string{"owner", "repo", "title", "head", "base"}}},
	map[string]any{"name": "merge_pull_request", "description": "Merge a pull request", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "pull_number": map[string]any{"type": "integer"}}, "required": []string{"owner", "repo", "pull_number"}}},
}

func rpc(w http.ResponseWriter, id any, result any) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func rpcError(w http.ResponseWriter, id any, code int, msg string) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}
func rpcErrorStatus(w http.ResponseWriter, status int, id any, code int, msg string) {
	writeJSON(w, status, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}
func (s *Forge) mcpRoute(w http.ResponseWriter, r *http.Request) {
	var m map[string]any
	if !decode(r, &m) {
		rpcErrorStatus(w, 400, nil, -32700, "Parse error")
		return
	}
	id := m["id"]
	method, _ := m["method"].(string)
	switch method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "forge-a2")
		version := "2026-07-28"
		if p, ok := m["params"].(map[string]any); ok {
			if v, ok := p["protocolVersion"].(string); ok {
				version = v
			}
		}
		rpc(w, id, map[string]any{"protocolVersion": version, "serverInfo": map[string]any{"name": "forge-a2", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}})
	case "notifications/initialized":
		w.WriteHeader(202)
	case "server/discover":
		rpc(w, id, map[string]any{"supportedVersions": []string{"2026-07-28", "2025-11-25"}, "serverInfo": map[string]any{"name": "forge-a2", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}})
	case "tools/list":
		rpc(w, id, map[string]any{"tools": mcpTools})
	case "tools/call":
		p, _ := m["params"].(map[string]any)
		name, _ := p["name"].(string)
		a, _ := p["arguments"].(map[string]any)
		if a == nil {
			a = map[string]any{}
		}
		res, ok := s.invokeTool(name, a)
		if !ok {
			rpcError(w, id, -32602, "Invalid tool or arguments")
			return
		}
		b, _ := json.Marshal(res)
		rpc(w, id, map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "structuredContent": res})
	default:
		rpcError(w, id, -32601, "Method not found")
	}
}
func stringOf(a map[string]any, k string) string { x, _ := a[k].(string); return x }
func (s *Forge) invokeTool(name string, a map[string]any) (any, bool) {
	owner, repo := stringOf(a, "owner"), stringOf(a, "repo")
	if full := stringOf(a, "full_name"); full != "" {
		p := strings.SplitN(full, "/", 2)
		if len(p) == 2 {
			owner, repo = p[0], p[1]
		}
	}
	switch strings.ToLower(name) {
	case "create_repo":
		n := stringOf(a, "name")
		if n == "" {
			return nil, false
		}
		x := s.putRepository(n, a["private"] == true, stringOf(a, "description"))
		return repositoryView(x), true
	case "get_repo":
		x := s.lookup(owner, repo)
		if x == nil {
			return nil, false
		}
		return repositoryView(x), true
	case "create_issue":
		x := s.lookup(owner, repo)
		if x == nil || stringOf(a, "title") == "" {
			return nil, false
		}
		x.Lock()
		defer x.Unlock()
		i, _ := s.fileTicket(x, stringOf(a, "title"), stringOf(a, "body"), nil)
		return ticketView(i), true
	case "list_issues":
		x := s.lookup(owner, repo)
		if x == nil {
			return nil, false
		}
		x.RLock()
		defer x.RUnlock()
		out := []any{}
		for _, i := range x.Tickets {
			if st := stringOf(a, "state"); st != "" && st != "all" && st != i.State {
				continue
			}
			out = append(out, ticketView(i))
		}
		return out, true
	case "create_pull_request":
		x := s.lookup(owner, repo)
		if x == nil {
			return nil, false
		}
		x.Lock()
		defer x.Unlock()
		p := s.openMerge(x, stringOf(a, "title"), stringOf(a, "body"), stringOf(a, "head"), stringOf(a, "base"))
		return mergeView(p), true
	case "merge_pull_request":
		x := s.lookup(owner, repo)
		n := int(a["pull_number"].(float64))
		if x == nil || n < 1 || n > len(x.Merges) {
			return nil, false
		}
		x.Lock()
		defer x.Unlock()
		sha, ok := s.landMerge(x, x.Merges[n-1])
		return map[string]any{"merged": ok, "sha": sha}, true
	}
	return nil, false
}
