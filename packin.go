// SPDX-License-Identifier: MIT

// Pack ingestion: inflate objects out of a pushed pack.
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

func ingestPack(pack []byte, x *Repository) error {
	if len(pack) < 12 || string(pack[:4]) != "PACK" {
		return fmt.Errorf("bad pack")
	}
	n := int(binary.BigEndian.Uint32(pack[8:12]))
	pos := 12
	for j := 0; j < n; j++ {
		if pos >= len(pack) {
			return fmt.Errorf("truncated pack object header")
		}
		c := pack[pos]
		pos++
		typ := int((c >> 4) & 7)
		size := int(c & 15)
		shift := 4
		for c&128 != 0 {
			if pos >= len(pack) {
				return fmt.Errorf("truncated pack object size")
			}
			c = pack[pos]
			pos++
			size |= int(c&127) << shift
			shift += 7
		}
		if typ == 6 {
			for {
				if pos >= len(pack) {
					return fmt.Errorf("truncated ofs-delta base")
				}
				c = pack[pos]
				pos++
				if c&128 == 0 {
					break
				}
			}
		} else if typ == 7 {
			if len(pack)-pos < 20 {
				return fmt.Errorf("truncated ref-delta base")
			}
			pos += 20
		}
		if pos >= len(pack) {
			return fmt.Errorf("truncated pack object data")
		}
		counted := &tallyReader{r: bytes.NewReader(pack[pos:])}
		var zr io.ReadCloser
		if pooled := zlibReaderPool.Get(); pooled != nil {
			zr = pooled.(io.ReadCloser)
			e := zr.(zlib.Resetter).Reset(counted, nil)
			if e != nil {
				return e
			}
		} else {
			var e error
			zr, e = zlib.NewReader(counted)
			if e != nil {
				return e
			}
		}
		data, e := io.ReadAll(zr)
		if e != nil {
			zr.Close()
			return e
		}
		zlibReaderPool.Put(zr)
		pos += counted.n
		if pos > len(pack) {
			return fmt.Errorf("truncated pack object data")
		}
		kind := map[int]string{1: "commit", 2: "tree", 3: "blob", 4: "tag"}[typ]
		if kind != "" && len(data) == size {
			x.keepObject(kind, data)
		}
	}
	return nil
}

type tallyReader struct {
	r io.Reader
	n int
}

func (c *tallyReader) Read(p []byte) (int, error) { n, e := c.r.Read(p); c.n += n; return n, e }
func (c *tallyReader) ReadByte() (byte, error) {
	var one [1]byte
	n, err := c.Read(one[:])
	if n == 1 {
		return one[0], nil
	}
	return 0, err
}
func inflatedBytes(b []byte) int {
	c := &tallyReader{r: bytes.NewReader(b)}
	z, e := zlib.NewReader(c)
	if e != nil {
		return 0
	}
	io.Copy(io.Discard, z)
	z.Close()
	return c.n
}
