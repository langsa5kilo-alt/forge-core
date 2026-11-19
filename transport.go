// SPDX-License-Identifier: MIT

// HTTP/1.1 session loop: request line match, header scan and one write per reply.
package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

type wireReply struct {
	c      net.Conn
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *wireReply) Header() http.Header { return w.header }
func (w *wireReply) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *wireReply) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.body.Write(p)
}

// setBody frames body (already placed after resCap reserved bytes) and writes the response once.
func (w *wireReply) setBody(b []byte, bodyAt int) error {
	root, body := b[:bodyAt:bodyAt], b[bodyAt:]
	if w.status == 0 {
		w.status = 200
	}
	if w.header.Get("Content-Length") == "" {
		w.header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	if w.header.Get("X-GitHub-Request-Id") == "" {
		w.header.Set("X-GitHub-Request-Id", "forge-a2")
	}
	head := renderHead(root, w.status, w.header)
	if len(head) > bodyAt {
		out := make([]byte, 0, len(head)+len(body))
		out = append(out, head...)
		out = append(out, body...)
		_, err := w.c.Write(out)
		return err
	}
	copy(b[len(head):], body)
	_, err := w.c.Write(b[:len(head)+len(body)])
	return err
}

func (w *wireReply) flush() error {
	if w.body.Len() == 0 {
		return w.setBody(make([]byte, resCap), resCap)
	}
	body := w.body.Bytes()
	b := make([]byte, resCap+len(body))
	copy(b[resCap:], body)
	return w.setBody(b, resCap)
}

// resCap is the room reserved before a body for the response head.
const resCap = 192

// renderHead writes the head into b (which holds exactly resCap bytes) and answers it as a slice.
func renderHead(b []byte, status int, h http.Header) []byte {
	b = b[:0]
	b = append(b, "HTTP/1.1 "...)
	b = strconv.AppendInt(b, int64(status), 10)
	b = append(b, ' ')
	b = append(b, http.StatusText(status)...)
	b = append(b, "\r\n"...)
	if len(h) == 0 {
		b = append(b, "Content-Type: application/json\r\nX-GitHub-Request-Id: forge-a2\r\n"...)
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		values := h[k]
		for _, v := range values {
			b = append(b, k...)
			b = append(b, ": "...)
			b = append(b, v...)
			b = append(b, "\r\n"...)
		}
	}
	b = append(b, "\r\n"...)
	return b
}

// ship puts one complete response (head already carrying Content-Length) on the wire.
func ship(c net.Conn, b []byte) error {
	_, err := c.Write(b)
	return err
}

func listenHTTP(ln net.Listener, handler http.Handler) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go serveSession(c, handler)
	}
}

// serveSession is the HTTP/1.1 loop: one buffered read per request, a byte-level request line match,
// header scanning only until the fields the fast paths need are found, and one Write per response.
func serveSession(c net.Conn, handler http.Handler) {
	defer c.Close()
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	store, _ := handler.(*Forge)
	expectedAuth := []byte(nil)
	if store != nil {
		expectedAuth = []byte("Bearer " + store.token)
	}
	buf := make([]byte, 0, 16<<10)
	var cachedPath []byte
	var cachedRepo *Repository
	for {
		headEnd := -1
		for headEnd < 0 {
			if i := bytes.Index(buf, []byte("\r\n\r\n")); i >= 0 {
				headEnd = i + 4
				break
			}
			if cap(buf)-len(buf) < 8<<10 {
				nb := make([]byte, len(buf), 2*cap(buf)+8<<10)
				copy(nb, buf)
				buf = nb
			}
			n, err := c.Read(buf[len(buf):cap(buf)])
			buf = buf[:len(buf)+n]
			if err != nil {
				return
			}
		}
		lineEnd := bytes.IndexByte(buf, '\n')
		if lineEnd < 1 {
			return
		}
		line := buf[:lineEnd]
		firstSpace := bytes.IndexByte(line, ' ')
		if firstSpace < 1 {
			return
		}
		rest := line[firstSpace+1:]
		secondSpace := bytes.IndexByte(rest, ' ')
		if secondSpace < 1 {
			return
		}
		method, path := line[:firstSpace], rest[:secondSpace]
		var authValue, connValue, lenValue []byte
		for i := lineEnd + 1; i < headEnd-2; {
			next := bytes.IndexByte(buf[i:headEnd-2], '\n')
			if next < 0 {
				return
			}
			hdr := bytes.TrimSuffix(buf[i:i+next], []byte("\r"))
			i += next + 1
			if len(hdr) == 0 {
				break
			}
			colon := bytes.IndexByte(hdr, ':')
			if colon < 1 {
				return
			}
			name := bytes.TrimSpace(hdr[:colon])
			value := bytes.TrimSpace(hdr[colon+1:])
			switch {
			case bytes.EqualFold(name, []byte("Authorization")):
				authValue = value
			case bytes.EqualFold(name, []byte("Connection")):
				connValue = value
			case bytes.EqualFold(name, []byte("Content-Length")):
				lenValue = value
			}
		}
		n := 0
		for _, digit := range lenValue {
			if digit < '0' || digit > '9' {
				return
			}
			n = n*10 + int(digit-'0')
		}
		connectionClose := bytes.EqualFold(connValue, []byte("close"))
		authOK := bytes.Equal(authValue, expectedAuth)
		if headEnd+n > len(buf) {
			oldLen := len(buf)
			if cap(buf) < headEnd+n {
				nb := make([]byte, oldLen, headEnd+n+8<<10)
				copy(nb, buf)
				buf = nb
			}
			buf = buf[:headEnd+n]
			if _, err := io.ReadFull(c, buf[oldLen:headEnd+n]); err != nil {
				return
			}
		}
		body := buf[headEnd : headEnd+n]
		consumed := headEnd + n
		var err error
		switch {
		case bytes.Equal(method, []byte("GET")) && bytes.Equal(path, []byte("/healthz")):
			err = ship(c, probeReply)
		case !authOK:
			err = ship(c, loginRequiredReply)
		default:
			m, owner, name, num, argsOK := matchHot(method, path)
			if m == routeNone {
				h := scanHeaders(buf, lineEnd, headEnd)
				err = store.serveStandard(c, h, method, path, body)
			} else if !argsOK {
				err = ship(c, missingReply)
			} else {
				if !bytes.Equal(path, cachedPath) {
					cachedPath = append(cachedPath[:0], path...)
					cachedRepo = store.repoOf(owner, name)
				}
				var response []byte
				ok := false
				if cachedRepo != nil {
					if m == routeIssueGet {
						response, ok = cachedRepo.ticketReply(num)
					} else {
						response, ok = store.quickTicketPostBytes(cachedRepo, body)
					}
				}
				if ok {
					err = ship(c, response)
				} else {
					err = ship(c, missingReply)
				}
			}
		}
		if err != nil {
			return
		}
		if connectionClose {
			return
		}
		if consumed >= len(buf) {
			buf = buf[:0]
		} else {
			copy(buf, buf[consumed:])
			buf = buf[:len(buf)-consumed]
		}
	}
}

func scanHeaders(buf []byte, lineEnd, headEnd int) http.Header {
	h := make(http.Header, 8)
	for i := lineEnd + 1; i < headEnd-2; {
		next := bytes.IndexByte(buf[i:headEnd-2], '\n')
		if next < 0 {
			break
		}
		hdr := bytes.TrimSuffix(buf[i:i+next], []byte("\r"))
		i += next + 1
		colon := bytes.IndexByte(hdr, ':')
		if colon < 1 {
			continue
		}
		name := bytes.TrimSpace(hdr[:colon])
		value := bytes.TrimSpace(hdr[colon+1:])
		h.Add(string(name), string(value))
	}
	return h
}

func (s *Forge) serveStandard(c net.Conn, h http.Header, method, path, body []byte) error {
	rw := &wireReply{c: c, header: make(http.Header, 8)}
	u, err := url.ParseRequestURI(string(path))
	if err != nil {
		return err
	}
	req := &http.Request{Method: string(method), URL: u, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: h, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Host: h.Get("Host"), RemoteAddr: c.RemoteAddr().String()}
	s.ServeHTTP(rw, req)
	return rw.flush()
}
