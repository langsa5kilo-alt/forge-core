// SPDX-License-Identifier: MIT

// Forge lifecycle: id minting, owner lookup and repository creation.
package main

import (
	"encoding/base64"
	"strconv"
)

func (s *Forge) mintID() int64 {
	s.seqMu.Lock()
	s.lastID++
	id := s.lastID
	s.seqMu.Unlock()
	return id
}
func (s *Forge) lookup(owner, name string) *Repository {
	if s.claimLogin && s.owner == "makefun-user" {
		s.Lock()
		if x := s.repositories[name]; x != nil && s.owner == "makefun-user" {
			s.owner = owner
			for _, repo := range s.repositories {
				repo.Owner = owner
			}
		}
		s.Unlock()
	}
	if owner != s.owner {
		return nil
	}
	s.RLock()
	x := s.repositories[name]
	s.RUnlock()
	return x
}
func (s *Forge) putRepository(name string, private bool, desc string) *Repository {
	s.Lock()
	defer s.Unlock()
	if existing := s.repositories[name]; existing != nil {
		return existing
	}
	id := s.mintID()
	x := &Repository{ID: id, Owner: s.owner, Name: name, Description: desc, Initial: "main", Private: private, Tips: map[string]string{}, Store: map[string]GitObject{}, Revisions: map[string]*Revision{}, Heads: map[string]map[string]BlobRef{"main": {}}, TagSet: map[string]Tag{}}
	s.repositories[name] = x
	return x
}
func repositoryView(x *Repository) map[string]any {
	return map[string]any{"id": x.ID, "node_id": globalID("Repository", x.ID), "name": x.Name, "full_name": x.Owner + "/" + x.Name, "private": x.Private, "description": x.Description, "default_branch": x.Initial, "owner": map[string]any{"login": x.Owner}}
}
func globalID(kind string, id int64) string {
	var raw [64]byte
	b := append(raw[:0], kind...)
	b = append(b, ':')
	b = strconv.AppendInt(b, id, 10)
	var encoded [88]byte
	n := base64.StdEncoding.EncodedLen(len(b))
	base64.StdEncoding.Encode(encoded[:n], b)
	return string(encoded[:n])
}
