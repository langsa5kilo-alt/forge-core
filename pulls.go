// SPDX-License-Identifier: MIT

// Pull requests: open, list, files and merge with conflict detection.
package main

import (
	"net/http"
	"sort"
	"strconv"
)

func mergeView(p *Merge) map[string]any {
	m := map[string]any{"id": p.ID, "node_id": globalID("PullRequest", p.ID), "number": p.Number, "title": p.Title, "body": p.Body, "state": p.State, "merged": p.Merged, "head": map[string]any{"ref": p.Head, "sha": p.SourceSHA}, "base": map[string]any{"ref": p.Base, "sha": p.TargetSHA}, "mergeable": true}
	if p.LandedAt != "" {
		m["merged_at"] = p.LandedAt
		m["merge_commit_sha"] = p.LandSHA
	}
	return m
}
func (s *Forge) openMerge(x *Repository, title, body, head, base string) *Merge {
	id := s.mintID()
	h := x.Tips["refs/heads/"+head]
	b := x.Tips["refs/heads/"+base]
	paths := []string{}
	hf, bf := x.Heads[head], x.Heads[base]
	for k, v := range hf {
		if z, ok := bf[k]; !ok || z.SHA != v.SHA {
			paths = append(paths, k)
		}
	}
	for k := range bf {
		if _, ok := hf[k]; !ok {
			paths = append(paths, k)
		}
	}
	sort.Strings(paths)
	p := &Merge{Number: len(x.Merges) + 1, ID: id, Title: title, Body: body, State: "open", Head: head, Base: base, SourceSHA: h, TargetSHA: b, Files: paths}
	x.Merges = append(x.Merges, p)
	return p
}
func (s *Forge) landMerge(x *Repository, p *Merge) (string, bool) {
	if p.Merged {
		return p.LandSHA, true
	}
	baseFiles := x.Heads[p.Base]
	headFiles := x.Heads[p.Head]
	baseTip := x.Tips["refs/heads/"+p.Base]
	head := x.Revisions[p.SourceSHA]
	base := x.Revisions[baseTip]
	if head != nil && base != nil && len(head.Parents) > 0 && head.Parents[0] != baseTip {
		for path, hf := range headFiles {
			if bf, ok := baseFiles[path]; ok {
				anc := map[string]BlobRef{}
				if pc := x.Revisions[head.Parents[0]]; pc != nil {
					anc = pc.Files
				}
				if af, ok := anc[path]; ok && bf.SHA != af.SHA && hf.SHA != af.SHA && hf.SHA != bf.SHA {
					return "", false
				}
			}
		}
	}
	merged := map[string]BlobRef{}
	for k, v := range baseFiles {
		merged[k] = v
	}
	for k, v := range headFiles {
		merged[k] = v
	}
	sha := x.commit(p.Base, p.Title, merged, []string{baseTip})
	p.State = "closed"
	p.Merged = true
	p.LandedAt = "2026-11-18T00:00:00Z"
	p.LandSHA = sha
	return sha, true
}
func (s *Forge) mergesRoute(w http.ResponseWriter, r *http.Request, x *Repository, parts []string) {
	if len(parts) == 0 {
		if r.Method == "POST" {
			var b struct{ Title, Body, Head, Base string }
			if !decode(r, &b) {
				writeRESTError(w, 400, "Invalid JSON")
				return
			}
			p := s.openMerge(x, b.Title, b.Body, b.Head, b.Base)
			writeREST(w, 201, mergeView(p))
			return
		}
		state := r.URL.Query().Get("state")
		if state == "" {
			state = "open"
		}
		out := []any{}
		for _, p := range x.Merges {
			if state != "all" && p.State != state {
				continue
			}
			out = append(out, mergeView(p))
		}
		writeREST(w, 200, out)
		return
	}
	n, _ := strconv.Atoi(parts[0])
	if n < 1 || n > len(x.Merges) {
		writeRESTError(w, 404, "Not Found")
		return
	}
	p := x.Merges[n-1]
	if len(parts) > 1 && parts[1] == "files" {
		out := []any{}
		for _, f := range p.Files {
			out = append(out, map[string]any{"filename": f, "status": "modified", "additions": 1, "deletions": 0, "changes": 1})
		}
		writeREST(w, 200, out)
		return
	}
	if len(parts) > 1 && parts[1] == "merge" {
		sha, ok := s.landMerge(x, p)
		if !ok {
			writeREST(w, 405, map[string]any{"message": "Pull Request is not mergeable", "merged": false})
			return
		}
		writeREST(w, 200, map[string]any{"sha": sha, "merged": true, "message": "Pull Request successfully merged"})
		return
	}
	writeREST(w, 200, mergeView(p))
}
