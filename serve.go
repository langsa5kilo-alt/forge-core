// SPDX-License-Identifier: MIT

// upload-pack: v2 ls-refs and fetch with the cached shallow clone pack.
package main

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Forge) serveFetch(w http.ResponseWriter, r *http.Request, x *Repository) {
	body, _ := io.ReadAll(r.Body)
	lines, _ := readSideband(body)
	cmd := ""
	wants := []string{}
	haves := map[string]bool{}
	hasShallow := false
	depth := 0
	prefixes := []string{}
	for _, l := range lines {
		z := strings.TrimSpace(string(l))
		if strings.HasPrefix(z, "command=") {
			cmd = strings.TrimPrefix(z, "command=")
		}
		if strings.HasPrefix(z, "want ") {
			wants = append(wants, strings.Fields(z)[1])
		}
		if strings.HasPrefix(z, "have ") {
			haves[strings.Fields(z)[1]] = true
		}
		if strings.HasPrefix(z, "deepen ") {
			depth, _ = strconv.Atoi(strings.Fields(z)[1])
		}
		if strings.HasPrefix(z, "shallow ") {
			hasShallow = true
		}
		if strings.HasPrefix(z, "ref-prefix ") {
			prefixes = append(prefixes, strings.TrimPrefix(z, "ref-prefix "))
		}
	}
	cacheKey := ""
	if cmd == "fetch" && depth > 0 && len(haves) == 0 {
		cacheKey = strings.Join(wants, ",") + ":" + strconv.Itoa(depth)
		if cached := x.recallPack(cacheKey); cached != nil {
			w.Write(cached)
			return
		}
		// Clone fast path: depth-1 fetch with no haves uses the prebuilt immutable pack.
		if depth == 1 && len(wants) == 1 {
			answer := x.clonePack.getOrBuild(x, wants, depth)
			w.Write(answer)
			return
		}
	}
	locked := true
	readLocked := cmd == "ls-refs"
	if readLocked {
		x.RLock()
	} else {
		x.Lock()
	}
	defer func() {
		if locked {
			if readLocked {
				x.RUnlock()
			} else {
				x.Unlock()
			}
		}
	}()
	if cmd == "ls-refs" {
		var b bytes.Buffer
		if len(x.Tips) == 0 {
			b.Write(framedLine("unborn HEAD symref-target:refs/heads/" + x.Initial + "\n"))
		} else {
			if sha := x.Tips["refs/heads/"+x.Initial]; sha != "" && len(prefixes) == 0 {
				b.Write(framedLine(sha + " HEAD symref-target:refs/heads/" + x.Initial + "\n"))
			}
			for _, ref := range orderedTips(x) {
				keep := len(prefixes) == 0
				for _, p := range prefixes {
					if strings.HasPrefix(ref, p) {
						keep = true
					}
				}
				if keep {
					line := x.Tips[ref] + " " + ref
					if strings.HasPrefix(ref, "refs/tags/") {
						if o := x.Store[x.Tips[ref]]; o.Kind == "tag" {
							if target := tagObject(o.Data); target != "" {
								line += " peeled:" + target
							}
						}
					}
					b.Write(framedLine(line + "\n"))
				}
			}
		}
		b.WriteString("0000")
		w.Write(b.Bytes())
		return
	}
	if len(wants) == 0 {
		w.Write([]byte("0000"))
		return
	}
	if cacheKey != "" {
		if cached := x.recallPack(cacheKey); cached != nil {
			x.Unlock()
			locked = false
			w.Write(cached)
			return
		}
	}
	if hasShallow {
		haves = map[string]bool{}
	}
	pack, shallow := assemblePack(x, wants, haves, depth)
	if cmd == "fetch" {
		var b bytes.Buffer
		if len(haves) > 0 {
			b.Write(framedLine("acknowledgments\n"))
			b.Write(framedLine("NAK\n"))
			b.WriteString("0001")
		}
		if shallow != "" {
			b.Write(framedLine("shallow-info\n"))
			b.Write(framedLine("shallow " + shallow + "\n"))
			b.WriteString("0001")
		}
		b.Write(framedLine("packfile\n"))
		for len(pack) > 0 {
			n := len(pack)
			if n > 65515 {
				n = 65515
			}
			b.Write(framedBytes(append([]byte{1}, pack[:n]...)))
			pack = pack[n:]
		}
		b.WriteString("0000")
		answer := b.Bytes()
		if cacheKey != "" {
			x.rememberPack(cacheKey, answer)
		}
		w.Write(answer)
		return
	}
	var b bytes.Buffer
	b.Write(framedLine("NAK\n"))
	for len(pack) > 0 {
		n := len(pack)
		if n > 65515 {
			n = 65515
		}
		b.Write(framedBytes(append([]byte{1}, pack[:n]...)))
		pack = pack[n:]
	}
	b.WriteString("0000")
	w.Write(b.Bytes())
}

// ticketReply answers issue n's ready 200 bytes without taking the repository lock.
