package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
)

type benchForge struct {
	store *Forge
	base  string
	ln    net.Listener
}

func newBenchForge(b *testing.B) *benchForge {
	b.Helper()
	s := &Forge{owner: "bench", token: "token", lastID: 1000, repositories: map[string]*Repository{}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	go listenHTTP(ln, s)
	b.Cleanup(func() { ln.Close() })
	return &benchForge{store: s, base: "http://" + ln.Addr().String(), ln: ln}
}

func benchRequest(b *testing.B, client *http.Client, method, target string, body []byte) []byte {
	b.Helper()
	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer token")
	if len(body) != 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		b.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		b.Fatal(err)
	}
	if resp.StatusCode >= 300 {
		b.Fatalf("%s %s: %s: %s", method, target, resp.Status, data)
	}
	return data
}

func seedIssueRepo(f *benchForge) *Repository {
	x := f.store.putRepository("issues", false, "")
	x.Lock()
	f.store.fileTicket(x, "profile issue", "profile body", nil)
	x.Unlock()
	return x
}

func BenchmarkRead(b *testing.B) {
	f := newBenchForge(b)
	seedIssueRepo(f)
	client := &http.Client{}
	target := f.base + "/api/v3/repos/bench/issues/issues/1"
	benchRequest(b, client, "GET", target, nil)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < 2000; i++ {
			benchRequest(b, client, "GET", target, nil)
		}
	}
}

func BenchmarkWrite(b *testing.B) {
	f := newBenchForge(b)
	f.store.putRepository("writes", false, "")
	client := &http.Client{}
	target := f.base + "/api/v3/repos/bench/writes/issues"
	body := []byte(`{"title":"benchmark issue","body":"benchmark body"}`)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < 200; i++ {
			benchRequest(b, client, "POST", target, body)
		}
	}
}

func benchPackObject(kind byte, data []byte) []byte {
	var out bytes.Buffer
	putObjectHeader(&out, kind, len(data))
	z := zlib.NewWriter(&out)
	z.Write(data)
	z.Close()
	return out.Bytes()
}

func oneObjectPack(kind byte, data []byte) []byte {
	var out bytes.Buffer
	out.WriteString("PACK")
	binary.Write(&out, binary.BigEndian, uint32(2))
	binary.Write(&out, binary.BigEndian, uint32(1))
	out.Write(benchPackObject(kind, data))
	return out.Bytes()
}

func BenchmarkPush(b *testing.B) {
	f := newBenchForge(b)
	f.store.putRepository("push", false, "")
	client := &http.Client{}
	target := f.base + "/bench/push.git/git-receive-pack"
	bodies := make([][]byte, 200)
	old := zeroOID
	for i := 0; i < 200; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 1024)
		sha := gitID("blob", data)
		cmd := fmt.Sprintf("%s %s refs/heads/main\x00report-status side-band-64k\n", old, sha)
		body := append(framedLine(cmd), []byte("0000")...)
		bodies[i] = append(body, oneObjectPack(3, data)...)
		old = sha
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, body := range bodies {
			benchRequest(b, client, "POST", target, body)
		}
	}
}

func seedCloneRepo(f *benchForge) (*Repository, string) {
	x := f.store.putRepository("clone", false, "")
	x.Lock()
	files := map[string]BlobRef{}
	parent := ""
	for i := 0; i < 200; i++ {
		data := fmt.Sprintf("revision %d", i)
		files["file.txt"] = BlobRef{Path: "file.txt", Data: data, SHA: blobID([]byte(data))}
		parent = x.commit("main", fmt.Sprintf("commit %d", i), files, []string{parent})
	}
	x.Unlock()
	return x, parent
}

func BenchmarkClone(b *testing.B) {
	f := newBenchForge(b)
	_, tip := seedCloneRepo(f)
	client := &http.Client{Transport: &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 100}}
	target := f.base + "/bench/clone.git/git-upload-pack"
	body := append(framedLine("command=fetch\n"), []byte("0001")...)
	body = append(body, framedLine("deepen 1\n")...)
	body = append(body, framedLine("want "+tip+"\n")...)
	body = append(body, framedLine("done\n")...)
	body = append(body, []byte("0000")...)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var wg sync.WaitGroup
		wg.Add(50)
		for i := 0; i < 50; i++ {
			go func() { defer wg.Done(); benchRequest(b, client, "POST", target, body) }()
		}
		wg.Wait()
	}
}
