// SPDX-License-Identifier: MIT

// Label, issue and branch REST handlers.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Forge) tagHandler(w http.ResponseWriter, r *http.Request, x *Repository) {
	var b Tag
	if !decode(r, &b) {
		writeRESTError(w, 400, "Invalid JSON")
		return
	}
	x.TagSet[b.Name] = b
	writeREST(w, 201, b)
}
func ticketView(i *Ticket) map[string]any {
	return map[string]any{"id": i.ID, "node_id": i.NodeID, "number": i.Number, "title": i.Title, "body": i.Body, "state": i.State, "labels": i.Labels, "comments": len(i.Comments)}
}
func (s *Forge) refreshTicket(x *Repository, i *Ticket) {
	i.Cached, _ = json.Marshal(ticketWire{i.ID, i.NodeID, i.Number, i.Title, i.Body, i.State, i.Labels, len(i.Comments)})
	i.Response = preframeReply(200, i.Cached)
}
func (s *Forge) fileTicket(x *Repository, title, body string, labels []string) (*Ticket, error) {
	ls := []Tag{}
	for _, n := range labels {
		l, ok := x.TagSet[n]
		if !ok {
			return nil, fmt.Errorf("unknown label")
		}
		ls = append(ls, l)
	}
	id := s.mintID()
	i := &Ticket{ID: id, NodeID: globalID("Issue", id), Number: len(x.Tickets) + 1, Title: title, Body: body, State: "open", Labels: ls}
	s.refreshTicket(x, i)
	x.Tickets = append(x.Tickets, i)
	s.republishTickets(x)
	return i, nil
}

// republishTickets replaces the atomic issue table with a new snapshot carrying all issues.
func (s *Forge) republishTickets(x *Repository) {
	t := &ticketSlots{bytes: make([][]byte, len(x.Tickets))}
	for idx, issue := range x.Tickets {
		t.bytes[idx] = issue.Response
	}
	x.ticketSlots.Store(t)
}
func (s *Forge) ticketsRoute(w http.ResponseWriter, r *http.Request, x *Repository, parts []string) {
	if len(parts) == 0 {
		if r.Method == "POST" {
			var b struct {
				Title, Body string
				Labels      []string
			}
			if !decode(r, &b) {
				writeRESTError(w, 400, "Invalid JSON")
				return
			}
			i, e := s.fileTicket(x, b.Title, b.Body, b.Labels)
			if e != nil {
				writeRESTError(w, 422, e.Error())
				return
			}
			writeREST(w, 201, ticketView(i))
			return
		}
		state := r.URL.Query().Get("state")
		out := []any{}
		for _, i := range x.Tickets {
			if state == "open" && i.State != "open" || state == "closed" && i.State != "closed" {
				continue
			}
			out = append(out, ticketView(i))
		}
		if r.URL.Query().Get("direction") == "desc" {
			for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
				out[i], out[j] = out[j], out[i]
			}
		}
		writeREST(w, 200, out)
		return
	}
	n, _ := strconv.Atoi(parts[0])
	if n < 1 || n > len(x.Tickets) {
		writeRESTError(w, 404, "Not Found")
		return
	}
	i := x.Tickets[n-1]
	if len(parts) > 1 && parts[1] == "comments" {
		if r.Method == "POST" {
			var b struct{ Body string }
			if !decode(r, &b) {
				writeRESTError(w, 400, "Invalid JSON")
				return
			}
			c := Remark{ID: s.mintID(), Body: b.Body}
			i.Comments = append(i.Comments, c)
			s.refreshTicket(x, i)
			s.republishTickets(x)
			writeREST(w, 201, c)
		} else {
			writeREST(w, 200, i.Comments)
		}
		return
	}
	if r.Method == "PATCH" {
		var b struct{ State string }
		if !decode(r, &b) {
			writeRESTError(w, 400, "Invalid JSON")
			return
		}
		if b.State != "" {
			i.State = b.State
		}
		s.refreshTicket(x, i)
		s.republishTickets(x)
	}
	writeREST(w, 200, ticketView(i))
}
func (s *Forge) gitRefsREST(w http.ResponseWriter, r *http.Request, x *Repository, p []string) {
	if len(p) == 1 && p[0] == "refs" && r.Method == "POST" {
		var b struct{ Ref, SHA string }
		if !decode(r, &b) {
			writeRESTError(w, 400, "Invalid JSON")
			return
		}
		x.Tips[b.Ref] = b.SHA
		br := strings.TrimPrefix(b.Ref, "refs/heads/")
		if c := x.Revisions[b.SHA]; c != nil {
			cp := map[string]BlobRef{}
			for k, v := range c.Files {
				cp[k] = v
			}
			x.Heads[br] = cp
		} else if base := x.Heads[x.Initial]; base != nil {
			cp := map[string]BlobRef{}
			for k, v := range base {
				cp[k] = v
			}
			x.Heads[br] = cp
		}
		writeREST(w, 201, map[string]any{"ref": b.Ref, "object": map[string]any{"sha": b.SHA}})
		return
	}
	writeRESTError(w, 404, "Not Found")
}
func (s *Forge) headsRoute(w http.ResponseWriter, r *http.Request, x *Repository) {
	out := []any{}
	for _, ref := range orderedTips(x) {
		if strings.HasPrefix(ref, "refs/heads/") {
			out = append(out, map[string]any{"name": strings.TrimPrefix(ref, "refs/heads/"), "commit": map[string]any{"sha": x.Tips[ref]}})
		}
	}
	writeREST(w, 200, out)
}
func (s *Forge) revisionREST(w http.ResponseWriter, r *http.Request, x *Repository) {
	br := r.URL.Query().Get("sha")
	if br == "" {
		br = x.Initial
	}
	sha := x.Tips["refs/heads/"+br]
	out := []any{}
	seen := map[string]bool{}
	for sha != "" && !seen[sha] {
		seen[sha] = true
		out = append(out, map[string]any{"sha": sha, "commit": map[string]any{"message": func() string {
			if c := x.Revisions[sha]; c != nil {
				return c.Message
			}
			return "commit"
		}()}})
		c := x.Revisions[sha]
		if c == nil || len(c.Parents) == 0 {
			break
		}
		sha = c.Parents[0]
	}
	writeREST(w, 200, out)
}
