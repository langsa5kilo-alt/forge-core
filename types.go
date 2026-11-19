// SPDX-License-Identifier: MIT

// The in-memory model: Forge, Repository, Ticket, Merge and the object types they store.
package main

import (
	"sync"
	"sync/atomic"
)

type GitObject struct {
	Kind string
	Data []byte
}
type BlobRef struct{ Path, Data, SHA string }
type Revision struct {
	SHA, Tree string
	Parents   []string
	Files     map[string]BlobRef
	Message   string
}
type Ticket struct {
	ID       int64    `json:"id"`
	NodeID   string   `json:"node_id"`
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	State    string   `json:"state"`
	Labels   []Tag    `json:"labels"`
	Comments []Remark `json:"-"`
	Cached   []byte   `json:"-"`
	Response []byte   `json:"-"`
}
type ticketWire struct {
	ID       int64  `json:"id"`
	NodeID   string `json:"node_id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	State    string `json:"state"`
	Labels   []Tag  `json:"labels"`
	Comments int    `json:"comments"`
}
type Tag struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}
type Remark struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}
type Merge struct {
	Number                                               int
	ID                                                   int64
	Title, Body, State, Head, Base, SourceSHA, TargetSHA string
	Merged                                               bool
	LandedAt, LandSHA                                    string
	Files                                                []string
}
type Repository struct {
	sync.RWMutex
	deltaCache                        atomic.Pointer[deltaMemo]
	clonePack                         bundleBuilder
	ID                                int64
	Owner, Name, Description, Initial string
	Private                           bool
	Tips                              map[string]string
	Store                             map[string]GitObject
	Revisions                         map[string]*Revision
	Heads                             map[string]map[string]BlobRef
	Tickets                           []*Ticket
	ticketSlots                       atomic.Pointer[ticketSlots]
	Merges                            []*Merge
	TagSet                            map[string]Tag
}

type deltaMemo struct{ entries map[string][]byte }

// ticketSlots is one repository's tickets as the hot GET path reads them: ticket n is entry n-1.
// The slice is replaced, never mutated, on every filing, so a reader needs no lock.
type ticketSlots struct{ bytes [][]byte }
type Forge struct {
	sync.RWMutex
	claimLogin   bool
	owner, token string
	lastID       int64
	seqMu        sync.Mutex
	repositories map[string]*Repository
}
