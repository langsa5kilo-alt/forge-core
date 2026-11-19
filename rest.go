// SPDX-License-Identifier: MIT

// REST v3 routes: root, meta, user repositories and the repository object.
package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (s *Forge) restRoute(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/v3")
	if (p == "" || p == "/") && r.Method == "GET" {
		w.Header().Set("X-OAuth-Scopes", "repo")
		writeREST(w, 200, map[string]any{"current_user_url": "/user"})
		return
	}
	if p == "/meta" && r.Method == "GET" {
		writeREST(w, 200, map[string]any{"installed_version": "forge-a2/1"})
		return
	}
	if p == "/user/repos" {
		s.userRepositories(w, r)
		return
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 3 || parts[0] != "repos" {
		writeRESTError(w, 404, "Not Found")
		return
	}
	owner, name := parts[1], parts[2]
	x := s.lookup(owner, name)
	if x == nil {
		writeRESTError(w, 404, "Not Found")
		return
	}
	x.Lock()
	defer x.Unlock()
	if len(parts) == 3 {
		switch r.Method {
		case "GET":
			writeREST(w, 200, repositoryView(x))
		case "PATCH":
			var b struct {
				Description string `json:"description"`
			}
			if !decode(r, &b) {
				writeRESTError(w, 400, "Invalid JSON")
				return
			}
			x.Description = b.Description
			writeREST(w, 200, repositoryView(x))
		case "DELETE":
			s.Lock()
			delete(s.repositories, name)
			s.Unlock()
			writeREST(w, 204, nil)
		default:
			writeRESTError(w, 404, "Not Found")
		}
		return
	}
	switch parts[3] {
	case "contents":
		s.blobsRoute(w, r, x, strings.Join(parts[4:], "/"))
	case "labels":
		s.tagHandler(w, r, x)
	case "issues":
		s.ticketsRoute(w, r, x, parts[4:])
	case "pulls":
		s.mergesRoute(w, r, x, parts[4:])
	case "git":
		s.gitRefsREST(w, r, x, parts[4:])
	case "branches":
		s.headsRoute(w, r, x)
	case "commits":
		s.revisionREST(w, r, x)
	default:
		writeRESTError(w, 404, "Not Found")
	}
}
func (s *Forge) userRepositories(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var b struct {
			Name, Description string
			Private           bool
		}
		if !decode(r, &b) {
			writeRESTError(w, 400, "Invalid JSON")
			return
		}
		x := s.putRepository(b.Name, b.Private, b.Description)
		writeREST(w, 201, repositoryView(x))
		return
	}
	s.RLock()
	a := make([]*Repository, 0, len(s.repositories))
	for _, x := range s.repositories {
		a = append(a, x)
	}
	s.RUnlock()
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if per < 1 {
		per = 30
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	lo := (page - 1) * per
	hi := lo + per
	if lo > len(a) {
		lo = len(a)
	}
	if hi > len(a) {
		hi = len(a)
	}
	out := make([]any, 0, hi-lo)
	for _, x := range a[lo:hi] {
		out = append(out, repositoryView(x))
	}
	if hi < len(a) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(page+1))
		u := *r.URL
		u.RawQuery = q.Encode()
		w.Header().Set("Link", "<http://"+r.Host+u.String()+">; rel=\"next\"")
	}
	writeREST(w, 200, out)
}
