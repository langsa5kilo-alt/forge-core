// SPDX-License-Identifier: MIT

// Pack assembly for clone and fetch, plus the pack caches.
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"net/url"
	"sort"
	"strings"
)

func (x *Repository) ticketReply(n int) ([]byte, bool) {
	t := x.ticketSlots.Load()
	if t == nil || n < 1 || n > len(t.bytes) {
		return nil, false
	}
	return t.bytes[n-1], true
}

func (x *Repository) resetDeltas() {
	// Only clear the general pack cache; the shallow clone pack is immutable once built and never cleared.
	x.deltaCache.Store(&deltaMemo{entries: map[string][]byte{}})
}

func (x *Repository) recallPack(key string) []byte {
	if cache := x.deltaCache.Load(); cache != nil {
		return cache.entries[key]
	}
	return nil
}

func (x *Repository) rememberPack(key string, data []byte) {
	entries := map[string][]byte{key: data}
	if old := x.deltaCache.Load(); old != nil {
		entries = make(map[string][]byte, len(old.entries)+1)
		for k, v := range old.entries {
			entries[k] = v
		}
		entries[key] = data
	}
	x.deltaCache.Store(&deltaMemo{entries: entries})
}
func tagObject(d []byte) string {
	for _, l := range strings.Split(string(d), "\n") {
		if strings.HasPrefix(l, "object ") {
			return strings.TrimPrefix(l, "object ")
		}
	}
	return ""
}
func assemblePack(x *Repository, wants []string, haves map[string]bool, depth int) ([]byte, string) {
	include := map[string]bool{}
	exclude := map[string]bool{}
	for h := range haves {
		collectObjects(x, h, exclude, 0, 0)
	}
	shallow := ""
	for _, w := range wants {
		collectObjects(x, w, include, depth, 0)
		if depth > 0 {
			z := w
			for i := 1; i < depth; i++ {
				c := loadRevision(x, z)
				if c == nil || len(c.Parents) == 0 {
					z = ""
					break
				}
				z = c.Parents[0]
			}
			if z != "" {
				c := loadRevision(x, z)
				if c != nil && len(c.Parents) > 0 {
					shallow = z
				}
			}
		}
	}
	ids := make([]string, 0, len(include))
	for id := range include {
		if !exclude[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var b bytes.Buffer
	b.WriteString("PACK")
	binary.Write(&b, binary.BigEndian, uint32(2))
	binary.Write(&b, binary.BigEndian, uint32(len(ids)))
	var zw *zlib.Writer
	for _, id := range ids {
		o := x.Store[id]
		typ := map[string]byte{"commit": 1, "tree": 2, "blob": 3, "tag": 4}[o.Kind]
		putObjectHeader(&b, typ, len(o.Data))
		if zw == nil {
			zw = zlib.NewWriter(&b)
		} else {
			zw.Reset(&b)
		}
		zw.Write(o.Data)
		zw.Close()
	}
	sum := sha1.Sum(b.Bytes())
	b.Write(sum[:])
	return b.Bytes(), shallow
}
func collectObjects(x *Repository, id string, set map[string]bool, depth, level int) {
	if id == "" || set[id] {
		return
	}
	o, ok := x.Store[id]
	if !ok {
		return
	}
	set[id] = true
	switch o.Kind {
	case "commit":
		c := loadRevision(x, id)
		if c == nil {
			return
		}
		collectObjects(x, c.Tree, set, 0, 0)
		if depth == 0 || level+1 < depth {
			for _, p := range c.Parents {
				collectObjects(x, p, set, depth, level+1)
			}
		}
	case "tree":
		d := o.Data
		for i := 0; i < len(d); {
			nul := bytes.IndexByte(d[i:], 0)
			if nul < 0 {
				return
			}
			nul += i
			if nul+21 > len(d) {
				return
			}
			collectObjects(x, hex.EncodeToString(d[nul+1:nul+21]), set, 0, 0)
			i = nul + 21
		}
	case "tag":
		collectObjects(x, tagObject(o.Data), set, depth, level)
	}
}
func putObjectHeader(w io.Writer, typ byte, size int) {
	c := byte(size&15) | (typ << 4)
	size >>= 4
	if size > 0 {
		c |= 128
	}
	w.Write([]byte{c})
	for size > 0 {
		c = byte(size & 127)
		size >>= 7
		if size > 0 {
			c |= 128
		}
		w.Write([]byte{c})
	}
}

var _ hash.Hash
var _ = url.URL{}
