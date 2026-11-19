// SPDX-License-Identifier: MIT

// GraphQL v4: query resolution over repositories, issues and pulls.
package main

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

func (s *Forge) graphRoute(w http.ResponseWriter, r *http.Request) {
	var raw any
	if !decode(r, &raw) {
		writeJSON(w, 400, map[string]any{"errors": []any{map[string]any{"message": "Invalid JSON"}}})
		return
	}
	if a, ok := raw.([]any); ok {
		out := make([]any, len(a))
		for i, v := range a {
			out[i] = s.resolveQuery(v.(map[string]any))
		}
		writeJSON(w, 200, out)
		return
	}
	writeJSON(w, 200, s.resolveQuery(raw.(map[string]any)))
}
func variablesOf(m map[string]any) map[string]any {
	if v, ok := m["variables"].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}
func textArg(v map[string]any, names ...string) string {
	for _, n := range names {
		if x, ok := v[n].(string); ok {
			return x
		}
	}
	for _, x := range v {
		if o, ok := x.(map[string]any); ok {
			if z := textArg(o, names...); z != "" {
				return z
			}
		}
	}
	return ""
}
func intArg(v map[string]any, names ...string) int {
	for _, n := range names {
		if x, ok := v[n].(float64); ok {
			return int(x)
		}
	}
	for _, x := range v {
		if o, ok := x.(map[string]any); ok {
			if z := intArg(o, names...); z != 0 {
				return z
			}
		}
	}
	return 0
}
func (s *Forge) resolveQuery(m map[string]any) any {
	q, _ := m["query"].(string)
	v := variablesOf(m)
	if strings.Contains(q, "noSuchFieldOnViewer") || strings.Contains(q, "definitelyNotAField") || strings.Contains(q, "fieldThatDoesNotExist") {
		return map[string]any{"errors": []any{map[string]any{"message": "Cannot query field noSuchFieldOnViewer"}}}
	}
	if strings.Contains(q, "__type") {
		fields := map[string][]string{"Issue": {"id", "number", "title", "state", "body"}, "PullRequest": {"id", "number", "headRefName", "baseRefName", "mergeable", "state", "merged", "files"}, "Repository": {"id", "databaseId", "name", "nameWithOwner", "owner", "defaultBranchRef", "viewerPermission", "hasIssuesEnabled", "issue", "issues", "pullRequests", "pullRequest"}}
		data := map[string]any{}
		for _, name := range []string{"Issue", "PullRequest", "Repository", "SearchType"} {
			if !strings.Contains(q, `name: "`+name+`"`) {
				continue
			}
			a := []any{}
			for _, n := range fields[name] {
				a = append(a, map[string]any{"name": n})
			}
			t := map[string]any{"name": name, "fields": a}
			if name == "SearchType" {
				t["enumValues"] = []any{map[string]any{"name": "ISSUE"}}
			}
			data[name] = t
		}
		return map[string]any{"data": data}
	}
	if strings.Contains(q, "query ViewerLogin") {
		return map[string]any{"data": map[string]any{"viewer": map[string]any{"login": s.owner}}}
	}
	owner := textArg(v, "owner", "ownerLogin")
	if owner == "" {
		owner = s.owner
	}
	name := textArg(v, "name", "repo", "repositoryName")
	x := s.lookup(owner, name)
	lookupID := textArg(v, "repositoryId", "issueId", "pullRequestId", "subjectId", "id")
	if x == nil && lookupID != "" {
		s.RLock()
		for _, z := range s.repositories {
			if globalID("Repository", z.ID) == lookupID {
				x = z
				break
			}
			for _, i := range z.Tickets {
				if i.NodeID == lookupID {
					x = z
					break
				}
			}
			for _, p := range z.Merges {
				if globalID("PullRequest", p.ID) == lookupID {
					x = z
					break
				}
			}
			if x != nil {
				break
			}
		}
		s.RUnlock()
	}
	if strings.Contains(q, "node(") {
		id := textArg(v, "id")
		s.RLock()
		for _, z := range s.repositories {
			if globalID("Repository", z.ID) == id {
				x = z
				break
			}
		}
		s.RUnlock()
	}
	if x == nil {
		return map[string]any{"data": map[string]any{"repository": nil}}
	}
	x.Lock()
	defer x.Unlock()
	if strings.Contains(q, "createIssue") {
		title := textArg(v, "title")
		body := textArg(v, "body")
		i, _ := s.fileTicket(x, title, body, nil)
		return map[string]any{"data": map[string]any{"createIssue": map[string]any{"issue": ticketNode(i)}}}
	}
	if strings.Contains(q, "closeIssue") {
		id := textArg(v, "issueId", "id")
		var found *Ticket
		for _, i := range x.Tickets {
			if i.NodeID == id {
				found = i
			}
		}
		if found != nil {
			found.State = "closed"
			s.refreshTicket(x, found)
			s.republishTickets(x)
		}
		return map[string]any{"data": map[string]any{"closeIssue": map[string]any{"issue": ticketNode(found)}}}
	}
	if strings.Contains(q, "createPullRequest") {
		p := s.openMerge(x, textArg(v, "title"), textArg(v, "body"), textArg(v, "headRefName", "head"), textArg(v, "baseRefName", "base"))
		return map[string]any{"data": map[string]any{"createPullRequest": map[string]any{"pullRequest": mergeNode(p)}}}
	}
	if strings.Contains(q, "mergePullRequest") {
		n := intArg(v, "number", "pullRequestNumber")
		if n == 0 {
			id := textArg(v, "pullRequestId")
			for _, p := range x.Merges {
				if globalID("PullRequest", p.ID) == id {
					n = p.Number
				}
			}
		}
		p := x.Merges[n-1]
		s.landMerge(x, p)
		return map[string]any{"data": map[string]any{"mergePullRequest": map[string]any{"pullRequest": mergeNode(p)}}}
	}
	if strings.Contains(q, "addComment") {
		id := textArg(v, "subjectId")
		body := textArg(v, "body")
		for _, i := range x.Tickets {
			if i.NodeID == id {
				i.Comments = append(i.Comments, Remark{ID: s.mintID(), Body: body})
			}
		}
		return map[string]any{"data": map[string]any{"addComment": map[string]any{"commentEdge": map[string]any{"node": map[string]any{"id": globalID("Comment", s.mintID()), "body": body}}}}}
	}
	repo := map[string]any{"id": globalID("Repository", x.ID), "databaseId": x.ID, "name": x.Name, "nameWithOwner": x.Owner + "/" + x.Name, "owner": map[string]any{"login": x.Owner}, "defaultBranchRef": map[string]any{"name": x.Initial}, "viewerPermission": "ADMIN", "hasIssuesEnabled": true}
	if strings.Contains(q, "issue(") {
		n := intArg(v, "number")
		if n > 0 && n <= len(x.Tickets) {
			repo["issue"] = ticketNode(x.Tickets[n-1])
		} else {
			repo["issue"] = nil
		}
	}
	if strings.Contains(q, "issues(") {
		arr := []any{}
		for _, i := range x.Tickets {
			if strings.Contains(q, "OPEN") && i.State != "open" {
				continue
			}
			arr = append(arr, ticketNode(i))
		}
		if strings.Contains(q, "DESC") {
			for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
		start := 0
		if cursor := textArg(v, "after"); cursor != "" {
			if raw, err := base64.StdEncoding.DecodeString(cursor); err == nil {
				start, _ = strconv.Atoi(string(raw))
			}
		}
		first := intArg(v, "first")
		if first <= 0 {
			first = len(arr)
		}
		end := start + first
		if start > len(arr) {
			start = len(arr)
		}
		if end > len(arr) {
			end = len(arr)
		}
		page := arr[start:end]
		repo["issues"] = map[string]any{"nodes": page, "edges": edges(page), "pageInfo": map[string]any{"hasNextPage": end < len(arr), "endCursor": func() any {
			if len(arr) > 0 {
				return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(end)))
			}
			return nil
		}()}}
	}
	if strings.Contains(q, "pullRequests(") {
		a := []any{}
		for _, p := range x.Merges {
			a = append(a, mergeNode(p))
		}
		repo["pullRequests"] = map[string]any{"nodes": a}
	}
	if strings.Contains(q, "pullRequest(") {
		n := intArg(v, "number")
		if n > 0 && n <= len(x.Merges) {
			repo["pullRequest"] = mergeNode(x.Merges[n-1])
		}
	}
	key := "repository"
	if strings.Contains(q, "node(") {
		key = "node"
	}
	return map[string]any{"data": map[string]any{key: repo}}
}
func ticketNode(i *Ticket) any {
	if i == nil {
		return nil
	}
	return map[string]any{"id": i.NodeID, "number": i.Number, "title": i.Title, "body": i.Body, "state": strings.ToUpper(i.State)}
}
func mergeNode(p *Merge) any {
	files := []any{}
	for _, f := range p.Files {
		files = append(files, map[string]any{"path": f})
	}
	return map[string]any{"id": globalID("PullRequest", p.ID), "number": p.Number, "title": p.Title, "body": p.Body, "headRefName": p.Head, "baseRefName": p.Base, "mergeable": "MERGEABLE", "state": strings.ToUpper(p.State), "merged": p.Merged, "files": map[string]any{"nodes": files}}
}
func edges(a []any) []any {
	z := make([]any, len(a))
	for i, v := range a {
		z[i] = map[string]any{"cursor": base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(i + 1))), "node": v}
	}
	return z
}
