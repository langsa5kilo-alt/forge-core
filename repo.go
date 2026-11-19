// SPDX-License-Identifier: MIT

// Object database: hashing, tree and revision writing.
package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func gitID(kind string, data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d\x00", kind, len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
func (x *Repository) keepObject(kind string, data []byte) string {
	id := gitID(kind, data)
	x.Store[id] = GitObject{kind, append([]byte(nil), data...)}
	return id
}
func writeTree(files map[string]BlobRef, x *Repository) string {
	type node struct {
		files map[string]BlobRef
		dirs  map[string]*node
	}
	root := &node{map[string]BlobRef{}, map[string]*node{}}
	for p, f := range files {
		seg := strings.Split(p, "/")
		n := root
		for _, d := range seg[:len(seg)-1] {
			if n.dirs[d] == nil {
				n.dirs[d] = &node{map[string]BlobRef{}, map[string]*node{}}
			}
			n = n.dirs[d]
		}
		n.files[seg[len(seg)-1]] = f
	}
	var build func(*node) string
	build = func(n *node) string {
		names := make([]string, 0, len(n.files)+len(n.dirs))
		for k := range n.files {
			names = append(names, k)
		}
		for k := range n.dirs {
			names = append(names, k)
		}
		sort.Strings(names)
		var b bytes.Buffer
		for _, k := range names {
			mode := "100644"
			var oid string
			if f, ok := n.files[k]; ok {
				oid = x.keepObject("blob", []byte(f.Data))
			} else {
				mode = "40000"
				oid = build(n.dirs[k])
			}
			fmt.Fprintf(&b, "%s %s\x00", mode, k)
			raw, _ := hex.DecodeString(oid)
			b.Write(raw)
		}
		return x.keepObject("tree", b.Bytes())
	}
	return build(root)
}
func (x *Repository) commit(branch, msg string, files map[string]BlobRef, parents []string) string {
	tree := writeTree(files, x)
	var b strings.Builder
	fmt.Fprintf(&b, "tree %s\n", tree)
	for _, p := range parents {
		if p != "" {
			fmt.Fprintf(&b, "parent %s\n", p)
		}
	}
	b.WriteString("author Forge <forge@example.com> 1700000000 +0000\ncommitter Forge <forge@example.com> 1700000000 +0000\n\n")
	b.WriteString(msg)
	b.WriteByte('\n')
	sha := x.keepObject("commit", []byte(b.String()))
	cp := map[string]BlobRef{}
	for k, v := range files {
		cp[k] = v
	}
	x.Revisions[sha] = &Revision{SHA: sha, Tree: tree, Parents: parents, Files: cp, Message: msg}
	x.Tips["refs/heads/"+branch] = sha
	x.Heads[branch] = cp
	x.resetDeltas()
	return sha
}
func blobID(data []byte) string { return gitID("blob", data) }
