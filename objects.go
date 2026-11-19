// SPDX-License-Identifier: MIT

// Revision materialisation: commits to trees to blobs.
package main

import (
	"bytes"
	"encoding/hex"
	"strings"
)

func loadRevision(x *Repository, sha string) *Revision {
	if c := x.Revisions[sha]; c != nil {
		return c
	}
	o, ok := x.Store[sha]
	if !ok || o.Kind != "commit" {
		return nil
	}
	text := string(o.Data)
	head := strings.SplitN(text, "\n\n", 2)
	lines := strings.Split(head[0], "\n")
	c := &Revision{SHA: sha, Files: map[string]BlobRef{}}
	for _, l := range lines {
		if strings.HasPrefix(l, "tree ") {
			c.Tree = strings.TrimPrefix(l, "tree ")
		} else if strings.HasPrefix(l, "parent ") {
			c.Parents = append(c.Parents, strings.TrimPrefix(l, "parent "))
		}
	}
	flattenTree(x, c.Tree, "", c.Files)
	if len(head) > 1 {
		c.Message = strings.TrimSpace(head[1])
	}
	x.Revisions[sha] = c
	return c
}
func flattenTree(x *Repository, sha, prefix string, out map[string]BlobRef) {
	o := x.Store[sha]
	d := o.Data
	for i := 0; i < len(d); {
		sp := bytes.IndexByte(d[i:], ' ')
		if sp < 0 {
			return
		}
		sp += i
		nul := bytes.IndexByte(d[sp+1:], 0)
		if nul < 0 {
			return
		}
		nul += sp + 1
		mode := string(d[i:sp])
		name := string(d[sp+1 : nul])
		if nul+21 > len(d) {
			return
		}
		id := hex.EncodeToString(d[nul+1 : nul+21])
		path := prefix + name
		if mode == "40000" {
			flattenTree(x, id, path+"/", out)
		} else {
			ob := x.Store[id]
			out[path] = BlobRef{Path: path, Data: string(ob.Data), SHA: id}
		}
		i = nul + 21
	}
}
