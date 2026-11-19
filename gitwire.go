// SPDX-License-Identifier: MIT

// Git pkt-line framing and repository lookup from a smart HTTP path.
package main

import (
	"fmt"
	"strconv"
)

func framedLine(data string) []byte { return []byte(fmt.Sprintf("%04x%s", len(data)+4, data)) }
func framedBytes(data []byte) []byte {
	h := []byte(fmt.Sprintf("%04x", len(data)+4))
	return append(h, data...)
}
func readSideband(b []byte) ([][]byte, int) {
	out := [][]byte{}
	i := 0
	for i+4 <= len(b) {
		n, e := strconv.ParseInt(string(b[i:i+4]), 16, 32)
		if e != nil {
			break
		}
		i += 4
		if n == 0 || n == 1 || n == 2 {
			out = append(out, nil)
			continue
		}
		n -= 4
		if i+int(n) > len(b) {
			break
		}
		out = append(out, b[i:i+int(n)])
		i += int(n)
	}
	return out, i
}
