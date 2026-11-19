// SPDX-License-Identifier: MIT

// Bearer token check and the login a token names.
package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

var probeReply = preframeReply(200, []byte(`{"ok":true}`))
var loginRequiredReply = preframeReply(401, []byte(`{"message":"Requires authentication"}`))
var missingReply = preframeReply(404, []byte(`{"message":"Not Found"}`))

func loginFromToken(t string) string {
	if p := strings.Split(t, "."); len(p) > 1 {
		if b, e := base64.RawURLEncoding.DecodeString(p[1]); e == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				for _, k := range []string{"login", "sub", "user"} {
					if x, ok := m[k].(string); ok && x != "" {
						return x
					}
				}
			}
		}
	}
	return "makefun-user"
}
