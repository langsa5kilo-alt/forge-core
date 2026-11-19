// SPDX-License-Identifier: MIT

// REST reply helpers and the hot ticket GET/POST fast paths.
package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Forge) quickTicketGet(w http.ResponseWriter, path string) bool {
	b, ok := s.ticketBody(path)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-GitHub-Request-Id", "forge-a2")
	w.WriteHeader(200)
	w.Write(b)
	return true
}
func (s *Forge) ticketBody(path string) ([]byte, bool) {
	p := strings.TrimPrefix(path, "/api/v3/repos/")
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[2] != "issues" {
		return nil, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, false
	}
	x := s.lookup(parts[0], parts[1])
	if x == nil {
		return nil, false
	}
	x.RLock()
	if n < 1 || n > len(x.Tickets) {
		x.RUnlock()
		return nil, false
	}
	b := x.Tickets[n-1].Cached
	x.RUnlock()
	return b, true
}
func (s *Forge) quickTicketPost(w http.ResponseWriter, r *http.Request) bool {
	p := strings.TrimPrefix(r.URL.Path, "/api/v3/repos/")
	parts := strings.Split(p, "/")
	if len(parts) != 3 || parts[2] != "issues" {
		return false
	}
	var body struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}
	if !decode(r, &body) {
		writeRESTError(w, 400, "Invalid JSON")
		return true
	}
	x := s.lookup(parts[0], parts[1])
	if x == nil {
		return false
	}
	x.Lock()
	i, err := s.fileTicket(x, body.Title, body.Body, body.Labels)
	x.Unlock()
	if err != nil {
		writeRESTError(w, 422, err.Error())
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-GitHub-Request-Id", "forge-a2")
	w.WriteHeader(201)
	w.Write(i.Cached)
	return true
}

// quickTicketPostBytes is the hot create path: the body is scanned field by field without a
// decoder (title, body, tags: strings, an optional tag list), and the answer is a 201 around the
// body the ticket cached when it was filed, so nothing is marshalled on the path.
func (s *Forge) quickTicketPostBytes(x *Repository, raw []byte) ([]byte, bool) {
	var title, body string
	var labels []string
	if !scanTicketPost(raw, &title, &body, &labels) {
		return badJSONReply, true
	}
	x.Lock()
	issue, err := s.fileTicket(x, title, body, labels)
	x.Unlock()
	if err != nil {
		return preframeReply(422, []byte(`{"message":"unknown label"}`)), true
	}
	return preframeReply(201, issue.Cached), true
}

var badJSONReply = preframeReply(400, []byte(`{"message":"Invalid JSON"}`))

// scanTicketPost reads the object GitHub clients send to POST .../issues. It shares the decoder's
// acceptance for well-formed bodies: top-level object, string title/body, tags as strings. A
// malformed body answers false and the caller replies 400, as the decoder would.
func scanTicketPost(raw []byte, title, body *string, labels *[]string) bool {
	if !json.Valid(raw) {
		return false
	}
	i := skipBlank(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return false
	}
	i++
	for {
		i = skipBlank(raw, i)
		if i >= len(raw) {
			return false
		}
		if raw[i] == '}' {
			return skipBlank(raw, i+1) == len(raw)
		}
		keyStart := i
		if raw[i] != '"' {
			return false
		}
		i = skipQuoted(raw, i)
		if i < 0 {
			return false
		}
		key := raw[keyStart:i]
		i = skipBlank(raw, i)
		if i >= len(raw) || raw[i] != ':' {
			return false
		}
		i = skipBlank(raw, i+1)
		switch {
		case equalLiteral(key, `"title"`):
			if i >= len(raw) || raw[i] != '"' {
				return false
			}
			end := skipQuoted(raw, i)
			if end < 0 || readQuoted(raw[i:end], title) != nil {
				return false
			}
			i = end
		case equalLiteral(key, `"body"`):
			if i >= len(raw) || raw[i] != '"' {
				return false
			}
			end := skipQuoted(raw, i)
			if end < 0 || readQuoted(raw[i:end], body) != nil {
				return false
			}
			i = end
		case equalLiteral(key, `"labels"`):
			if i >= len(raw) || raw[i] != '[' {
				return false
			}
			i++
			for {
				i = skipBlank(raw, i)
				if i >= len(raw) {
					return false
				}
				if raw[i] == ']' {
					i++
					break
				}
				if raw[i] != '"' {
					return false
				}
				end := skipQuoted(raw, i)
				if end < 0 {
					return false
				}
				var l string
				if readQuoted(raw[i:end], &l) != nil {
					return false
				}
				*labels = append(*labels, l)
				i = skipBlank(raw, end)
				if i < len(raw) && raw[i] == ',' {
					i++
				}
			}
		default:
			var end int
			end, i = skipAnything(raw, i)
			if end < 0 {
				return false
			}
		}
		i = skipBlank(raw, i)
		if i < len(raw) && raw[i] == ',' {
			i++
			continue
		}
		if i < len(raw) && raw[i] == '}' {
			return skipBlank(raw, i+1) == len(raw)
		}
		return false
	}
}

func equalLiteral(a []byte, lit string) bool { return len(a) == len(lit) && string(a) == lit }

func skipBlank(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// skipQuoted answers the index just past the closing quote of the string starting at i, or -1.
func skipQuoted(b []byte, i int) int {
	if i >= len(b) || b[i] != '"' {
		return -1
	}
	i++
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return -1
}

// readQuoted decodes the JSON string b (quotes included) into s.
func readQuoted(b []byte, s *string) error {
	return json.Unmarshal(b, s)
}

// skipAnything answers the index just past one JSON value at i (used for fields the route ignores).
func skipAnything(b []byte, i int) (int, int) {
	if i >= len(b) {
		return -1, i
	}
	switch b[i] {
	case '"':
		e := skipQuoted(b, i)
		return e, e
	case '{', '[':
		depth := 0
		for j := i; j < len(b); j++ {
			switch b[j] {
			case '"':
				j = skipQuoted(b, j) - 1
				if j < 0 {
					return -1, i
				}
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, j + 1
				}
			}
		}
		return -1, i
	default:
		j := i
		for j < len(b) && b[j] != ',' && b[j] != '}' && b[j] != ']' && b[j] != ' ' && b[j] != '\n' && b[j] != '\r' && b[j] != '\t' {
			j++
		}
		if j == i {
			return -1, i
		}
		return j, j
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != 204 {
		json.NewEncoder(w).Encode(v)
	}
}
func writeREST(w http.ResponseWriter, status int, v any) {
	w.Header().Set("X-GitHub-Request-Id", "forge-a2")
	writeJSON(w, status, v)
}
func writeRESTError(w http.ResponseWriter, status int, msg string) {
	writeREST(w, status, map[string]any{"message": msg})
}
func decode(r *http.Request, v any) bool { d := json.NewDecoder(r.Body); return d.Decode(v) == nil }
