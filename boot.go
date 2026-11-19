// SPDX-License-Identifier: MIT

// Process entry: bind an ephemeral port, register with the suite, serve until done.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	token := os.Getenv("MAKEFUN_SHOP_TOKEN")
	owner := loginFromToken(token)
	s := &Forge{owner: owner, token: token, lastID: 1000, repositories: make(map[string]*Repository), claimLogin: true}
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go listenHTTP(ln, s)
	env := os.Getenv("MAKEFUN_ENV_URL")
	body, _ := json.Marshal(map[string]int{"port": port})
	for {
		req, _ := http.NewRequest("POST", env+"/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(req)
		if e == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 300 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	for {
		resp, e := http.Get(env + "/done")
		if e == nil {
			var v struct {
				Done bool `json:"done"`
			}
			json.NewDecoder(resp.Body).Decode(&v)
			resp.Body.Close()
			if v.Done {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	ln.Close()
}
