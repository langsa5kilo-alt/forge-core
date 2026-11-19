// SPDX-License-Identifier: MIT

// Front door: auth, hot ticket routes, then REST, GraphQL, MCP and git.
package main

import (
	"net/http"
	"strings"
)

func (s *Forge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		if strings.HasPrefix(r.URL.Path, "/api/v3") {
			writeRESTError(w, 401, "Requires authentication")
		} else {
			writeJSON(w, 401, map[string]any{"message": "Requires authentication"})
		}
		return
	}
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v3/repos/") && s.quickTicketGet(w, r.URL.Path) {
		return
	}
	if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v3/repos/") && s.quickTicketPost(w, r) {
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v3") {
		s.restRoute(w, r)
		return
	}
	if r.URL.Path == "/api/graphql" {
		s.graphRoute(w, r)
		return
	}
	if r.URL.Path == "/mcp" {
		s.mcpRoute(w, r)
		return
	}
	if strings.Contains(r.URL.Path, ".git/") {
		s.gitRoute(w, r)
		return
	}
	writeJSON(w, 404, map[string]any{"message": "Not Found"})
}
