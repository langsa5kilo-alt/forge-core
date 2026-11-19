// SPDX-License-Identifier: MIT
package main

import "bytes"

// Byte-level route table for the two timed REST paths. A request line is matched without
// allocations: the method, the /api/v3/repos/ prefix, the "issues" shape and a decimal issue
// number (or the bare create path) are checked in place and the issue number is built inline.
type hotRoute int

const (
	routeNone hotRoute = iota
	routeIssueGet
	routeIssueCreate
)

const apiRepos = "/api/v3/repos/"

// matchHot answers the route and, for issue reads, the issue number; owner and name are the
// request-line slices the repo lookup keys on. argsOK is false when the path has the handled shape
// but the number is not a plain positive decimal.
func matchHot(method, path []byte) (route hotRoute, owner, name []byte, n int, argsOK bool) {
	argsOK = true
	if bytes.HasPrefix(path, []byte(apiRepos)) {
		p := path[len(apiRepos):]
		slash := bytes.IndexByte(p, '/')
		if slash >= 1 {
			owner = p[:slash]
			p = p[slash+1:]
			slash = bytes.IndexByte(p, '/')
			if slash >= 1 {
				name = p[:slash]
				p = p[slash+1:]
				if !bytes.HasPrefix(p, []byte("issues")) {
					return routeNone, nil, nil, 0, true
				}
				p = p[len("issues"):]
				if len(p) == 0 {
					if bytes.Equal(method, []byte("POST")) {
						return routeIssueCreate, owner, name, 0, true
					}
					return routeNone, nil, nil, 0, true
				}
				if len(p) > 1 && p[0] == '/' && bytes.Equal(method, []byte("GET")) {
					p = p[1:]
					for _, d := range p {
						if d < '0' || d > '9' {
							if d == '/' {
								return routeNone, nil, nil, 0, true
							}
							return routeIssueGet, owner, name, 0, false
						}
						n = n*10 + int(d-'0')
						if n > 1<<24 {
							return routeIssueGet, owner, name, 0, false
						}
					}
					if n == 0 {
						return routeIssueGet, owner, name, 0, false
					}
					return routeIssueGet, owner, name, n, true
				}
			}
		}
	}
	return routeNone, nil, nil, 0, true
}

// repoOf is Forge.lookup for request-line slices: it builds the two map keys and defers to lookup,
// so the owner rule (and the one-time login claim) stays exactly one implementation.
func (s *Forge) repoOf(owner, name []byte) *Repository {
	return s.lookup(string(owner), string(name))
}

// serveHot answers a matched hot route from precomputed bytes. Each ticket keeps its full 200
// response framed when it is filed (Response): a read is a number-to-slice lookup and a write
// frames the cached 201 body into a fresh 201 reply.
func (s *Forge) serveHot(m hotRoute, owner, name []byte, n int, body []byte) ([]byte, bool) {
	x := s.repoOf(owner, name)
	return s.serveHotRepo(m, x, n, body)
}

func (s *Forge) serveHotRepo(m hotRoute, x *Repository, n int, body []byte) ([]byte, bool) {
	if x == nil {
		return nil, false
	}
	if m == routeIssueGet {
		return x.ticketReply(n)
	}
	return s.quickTicketPostBytes(x, body)
}
