// SPDX-License-Identifier: MIT
package main

import (
	"sync"
	"sync/atomic"
)

// bundleBuilder holds the shallow clone pack: built once on first request with double-checked
// locking, then served read-only from an atomic pointer. The pack bytes are immutable after
// construction, so clone requests never take the repository write lock.
type bundleBuilder struct {
	once   sync.Once
	mu     sync.Mutex
	packed atomic.Pointer[[]byte]
}

// getOrBuild answers the retained pack or builds it exactly once. Double-checked locking: the fast
// path (cached != nil) reads the atomic pointer without acquiring mu, the slow path (build) holds
// mu and checks again before building. The repository read lock is taken only during the build
// itself, never on subsequent reads.
func (b *bundleBuilder) getOrBuild(x *Repository, wants []string, depth int) []byte {
	if cached := b.packed.Load(); cached != nil {
		return *cached
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if cached := b.packed.Load(); cached != nil {
		return *cached
	}
	x.RLock()
	pack, shallow := assemblePack(x, wants, nil, depth)
	x.RUnlock()
	var answer []byte
	if depth > 0 {
		answer = wrapFetchPack(pack, shallow)
	} else {
		answer = wrapV1Pack(pack)
	}
	b.packed.Store(&answer)
	return answer
}

// wrapFetchPack frames a pack for git protocol v2 fetch (with shallow-info when depth > 0).
func wrapFetchPack(pack []byte, shallow string) []byte {
	capacity := 128 + len(pack) + (len(pack)/65515+1)*8
	buf := make([]byte, 0, capacity)
	if shallow != "" {
		buf = append(buf, framedLine("shallow-info\n")...)
		buf = append(buf, framedLine("shallow "+shallow+"\n")...)
		buf = append(buf, "0001"...)
	}
	buf = append(buf, framedLine("packfile\n")...)
	for len(pack) > 0 {
		chunk := pack
		if len(chunk) > 65515 {
			chunk = pack[:65515]
		}
		buf = append(buf, framedBytes(append([]byte{1}, chunk...))...)
		pack = pack[len(chunk):]
	}
	buf = append(buf, "0000"...)
	return buf
}

// wrapV1Pack frames a pack for git protocol v1 upload-pack (NAK + sideband).
func wrapV1Pack(pack []byte) []byte {
	capacity := 64 + len(pack) + (len(pack)/65515+1)*8
	buf := make([]byte, 0, capacity)
	buf = append(buf, framedLine("NAK\n")...)
	for len(pack) > 0 {
		chunk := pack
		if len(chunk) > 65515 {
			chunk = pack[:65515]
		}
		buf = append(buf, framedBytes(append([]byte{1}, chunk...))...)
		pack = pack[len(chunk):]
	}
	buf = append(buf, "0000"...)
	return buf
}
