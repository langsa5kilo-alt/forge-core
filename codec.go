// SPDX-License-Identifier: MIT

// Response framing: preframed replies and the pooled buffers shared by codecs and packs.
package main

import (
	"net/http"
	"strconv"
	"sync"
)

var responseBufferPool = sync.Pool{New: func() any { b := make([]byte, 0, 512); return &b }}
var zlibReaderPool sync.Pool
var frameBufferPool = sync.Pool{New: func() any { b := make([]byte, 0, 64<<10); return &b }}

func preframeReply(status int, body []byte) []byte {
	b := make([]byte, 0, len(body)+160)
	b = append(b, "HTTP/1.1 "...)
	b = strconv.AppendInt(b, int64(status), 10)
	b = append(b, ' ')
	b = append(b, http.StatusText(status)...)
	b = append(b, "\r\nContent-Type: application/json\r\nX-GitHub-Request-Id: forge-a2\r\nContent-Length: "...)
	b = strconv.AppendInt(b, int64(len(body)), 10)
	b = append(b, "\r\n\r\n"...)
	b = append(b, body...)
	return b
}

// wireReply collects one response and writes it as a single framed buffer. The REST handlers
// hand their bytes to setBody, which frames status line, headers and body and writes them once.
