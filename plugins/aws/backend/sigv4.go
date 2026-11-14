// AWS Signature Version 4, without build tags so it is testable natively.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Credentials are an access key and its secret.
type Credentials struct {
	AccessKey, SecretKey string
}

// SignRequest is what Signature Version 4 signs. Headers must contain every
// header that will be sent and signed (X-Amz-Date included) except Host,
// which comes from the Host field.
type SignRequest struct {
	Method   string
	Host     string
	Path     string
	RawQuery string
	Headers  map[string]string
	Body     string
}

const amzDate = "20060102T150405Z"

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// uriEncode encodes like RFC 3986: unreserved characters stay, everything
// else becomes %XX (upper case).
func uriEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

// canonicalQuery sorts the parameters by name, then value, and encodes them.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type kv struct{ k, v string }
	var params []kv
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		params = append(params, kv{uriEncode(k, false), uriEncode(v, false)})
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].k != params[j].k {
			return params[i].k < params[j].k
		}
		return params[i].v < params[j].v
	})
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

// canonicalHeaders returns the canonical header block and the signed header
// list: names in lower case, sorted, values trimmed with inner runs of
// spaces collapsed.
func canonicalHeaders(host string, headers map[string]string) (string, string) {
	all := map[string]string{"host": host}
	for k, v := range headers {
		all[strings.ToLower(strings.TrimSpace(k))] = strings.Join(strings.Fields(v), " ")
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k + ":" + all[k] + "\n")
	}
	return b.String(), strings.Join(names, ";")
}

// canonicalRequest builds the canonical request (step 1).
func canonicalRequest(r SignRequest) (string, string) {
	path := r.Path
	if path == "" {
		path = "/"
	}
	headers, signed := canonicalHeaders(r.Host, r.Headers)
	return strings.Join([]string{
		r.Method,
		uriEncode(path, true),
		canonicalQuery(r.RawQuery),
		headers,
		signed,
		sha256Hex(r.Body),
	}, "\n"), signed
}

// stringToSign builds the string to sign (step 2).
func stringToSign(canonical string, t time.Time, scope string) string {
	return "AWS4-HMAC-SHA256\n" + t.UTC().Format(amzDate) + "\n" + scope + "\n" + sha256Hex(canonical)
}

// signingKey derives the key for one day, region and service (step 3).
func signingKey(secret string, t time.Time, region, service string) []byte {
	k := hmacSHA256([]byte("AWS4"+secret), t.UTC().Format("20060102"))
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	return hmacSHA256(k, "aws4_request")
}

// authorization signs r and returns the Authorization header value.
func authorization(r SignRequest, c Credentials, region, service string, t time.Time) string {
	scope := t.UTC().Format("20060102") + "/" + region + "/" + service + "/aws4_request"
	canonical, signed := canonicalRequest(r)
	sig := hex.EncodeToString(hmacSHA256(signingKey(c.SecretKey, t, region, service), stringToSign(canonical, t, scope)))
	return "AWS4-HMAC-SHA256 Credential=" + c.AccessKey + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + sig
}
