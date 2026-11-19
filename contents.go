// SPDX-License-Identifier: MIT

// Contents API: read, create, update and delete files through commits.
package main

import (
	"encoding/base64"
	"net/http"
	"strings"
)

func (s *Forge) blobsRoute(w http.ResponseWriter, r *http.Request, x *Repository, path string) {
	branch := r.URL.Query().Get("ref")
	if branch == "" {
		branch = x.Initial
	}
	files := x.Heads[branch]
	if files == nil {
		writeRESTError(w, 404, "Not Found")
		return
	}
	if r.Method == "GET" {
		if f, ok := files[path]; ok {
			writeREST(w, 200, map[string]any{"type": "file", "name": baseName(path), "path": path, "sha": f.SHA, "size": len(f.Data), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(f.Data))})
			return
		}
		prefix := strings.TrimSuffix(path, "/") + "/"
		seen := map[string]bool{}
		out := []any{}
		for p, f := range files {
			if strings.HasPrefix(p, prefix) {
				tail := strings.TrimPrefix(p, prefix)
				first := strings.Split(tail, "/")[0]
				full := prefix + first
				if seen[full] {
					continue
				}
				seen[full] = true
				if strings.Contains(tail, "/") {
					out = append(out, map[string]any{"type": "dir", "name": first, "path": full, "sha": f.SHA})
				} else {
					out = append(out, map[string]any{"type": "file", "name": first, "path": full, "sha": f.SHA})
				}
			}
		}
		if len(out) == 0 {
			writeRESTError(w, 404, "Not Found")
		} else {
			writeREST(w, 200, out)
		}
		return
	}
	var b struct{ Message, Content, SHA, Branch string }
	if !decode(r, &b) {
		writeRESTError(w, 400, "Invalid JSON")
		return
	}
	if b.Branch != "" {
		branch = b.Branch
	}
	files = x.Heads[branch]
	if files == nil {
		base := x.Heads[x.Initial]
		files = map[string]BlobRef{}
		for k, v := range base {
			files[k] = v
		}
	}
	copyFiles := make(map[string]BlobRef, len(files))
	for k, v := range files {
		copyFiles[k] = v
	}
	files = copyFiles
	old, exists := files[path]
	if r.Method == "DELETE" {
		if !exists {
			writeRESTError(w, 404, "Not Found")
			return
		}
		delete(files, path)
	} else {
		if b.SHA != "" && (!exists || b.SHA != old.SHA) {
			writeRESTError(w, 409, "sha does not match")
			return
		}
		raw, e := base64.StdEncoding.DecodeString(b.Content)
		if e != nil {
			writeRESTError(w, 422, "Invalid content")
			return
		}
		files[path] = BlobRef{Path: path, Data: string(raw), SHA: blobID(raw)}
	}
	parent := x.Tips["refs/heads/"+branch]
	sha := x.commit(branch, b.Message, files, []string{parent})
	if r.Method == "DELETE" {
		writeREST(w, 200, map[string]any{"commit": map[string]any{"sha": sha}})
	} else {
		status := 201
		if exists {
			status = 200
		}
		f := files[path]
		writeREST(w, status, map[string]any{"content": map[string]any{"sha": f.SHA, "encoding": "base64"}, "commit": map[string]any{"sha": sha}})
	}
}
func baseName(p string) string { a := strings.Split(p, "/"); return a[len(a)-1] }
