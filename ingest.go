// SPDX-License-Identifier: MIT

// receive-pack: ref commands, fast-forward checks and report status.
package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

func (s *Forge) ingestPush(w http.ResponseWriter, r *http.Request, x *Repository) error {
	body, _ := io.ReadAll(r.Body)
	lines, off := readSideband(body)
	type cmd struct{ old, new, ref string }
	cmds := []cmd{}
	for _, l := range lines {
		if len(l) == 0 {
			break
		}
		z := strings.TrimSpace(strings.SplitN(string(l), "\x00", 2)[0])
		p := strings.Fields(z)
		if len(p) >= 3 {
			cmds = append(cmds, cmd{p[0], p[1], p[2]})
		}
	}
	if off < len(body) && bytes.HasPrefix(body[off:], []byte("PACK")) {
		if err := ingestPack(body[off:], x); err != nil {
			return err
		}
	}
	var report bytes.Buffer
	report.Write(framedLine("unpack ok\n"))
	for _, c := range cmds {
		cur := x.Tips[c.ref]
		hadHeads := false
		for ref := range x.Tips {
			if strings.HasPrefix(ref, "refs/heads/") {
				hadHeads = true
				break
			}
		}
		ok := c.old == zeroOID && cur == "" || c.old == cur
		if ok && c.new != zeroOID && cur != "" && !reachesParent(x, cur, c.new) {
			ok = false
		}
		if ok {
			if c.new == zeroOID {
				delete(x.Tips, c.ref)
			} else {
				x.Tips[c.ref] = c.new
				if strings.HasPrefix(c.ref, "refs/heads/") {
					br := strings.TrimPrefix(c.ref, "refs/heads/")
					if !hadHeads {
						x.Initial = br
					}
					if cm := loadRevision(x, c.new); cm != nil {
						x.Heads[br] = cm.Files
					}
				}
			}
			report.Write(framedLine("ok " + c.ref + "\n"))
		} else {
			report.Write(framedLine("ng " + c.ref + " non-fast-forward\n"))
		}
	}
	if len(cmds) > 0 {
		x.resetDeltas()
	}
	report.WriteString("0000")
	var side bytes.Buffer
	side.Write(framedBytes(append([]byte{1}, report.Bytes()...)))
	side.WriteString("0000")
	w.Write(side.Bytes())
	return nil
}
func reachesParent(x *Repository, old, tip string) bool {
	seen := map[string]bool{}
	q := []string{tip}
	for len(q) > 0 {
		z := q[0]
		q = q[1:]
		if z == old {
			return true
		}
		if seen[z] {
			continue
		}
		seen[z] = true
		c := loadRevision(x, z)
		if c != nil {
			q = append(q, c.Parents...)
		}
	}
	return false
}
