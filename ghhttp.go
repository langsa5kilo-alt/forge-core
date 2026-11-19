// SPDX-License-Identifier: MIT

// Git smart HTTP: capability advertisements and push/fetch dispatch.
package main

import (
	"bytes"
	"net/http"
	"sort"
	"strings"
)

func repositoryForGit(s *Forge, path string) (*Repository, string) {
	p := strings.Split(strings.Trim(path, "/"), "/")
	if len(p) < 3 {
		return nil, ""
	}
	name := strings.TrimSuffix(p[1], ".git")
	return s.lookup(p[0], name), p[len(p)-1]
}
func (s *Forge) gitRoute(w http.ResponseWriter, r *http.Request) {
	x, action := repositoryForGit(s, r.URL.Path)
	if x == nil {
		http.NotFound(w, r)
		return
	}
	if action == "refs" {
		x.RLock()
		defer x.RUnlock()
		service := r.URL.Query().Get("service")
		if service == "git-upload-pack" && strings.Contains(r.Header.Get("Git-Protocol"), "version=2") {
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			var b bytes.Buffer
			b.Write(framedLine("version 2\n"))
			b.Write(framedLine("agent=forge-a2\n"))
			b.Write(framedLine("ls-refs=unborn\n"))
			b.Write(framedLine("fetch=shallow wait-for-done\n"))
			b.WriteString("0000")
			w.Write(b.Bytes())
			return
		}
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		var b bytes.Buffer
		b.Write(framedLine("# service=" + service + "\n"))
		b.WriteString("0000")
		caps := "report-status side-band-64k delete-refs ofs-delta"
		if service == "git-upload-pack" {
			caps = "multi_ack_detailed side-band-64k thin-pack ofs-delta shallow symref=HEAD:refs/heads/" + x.Initial
		}
		refs := orderedTips(x)
		if len(refs) == 0 {
			b.Write(framedLine(zeroOID + " capabilities^{}\x00" + caps + "\n"))
		} else {
			for i, ref := range refs {
				line := x.Tips[ref] + " " + ref
				if i == 0 {
					line += "\x00" + caps
				}
				b.Write(framedLine(line + "\n"))
			}
		}
		b.WriteString("0000")
		w.Write(b.Bytes())
		return
	}
	if action == "git-receive-pack" {
		x.Lock()
		defer x.Unlock()
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		if err := s.ingestPush(w, r, x); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	if action == "git-upload-pack" {
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		s.serveFetch(w, r, x)
		return
	}
	http.NotFound(w, r)
}
func orderedTips(x *Repository) []string {
	a := make([]string, 0, len(x.Tips))
	for k := range x.Tips {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
