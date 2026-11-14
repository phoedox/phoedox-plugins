package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"strings"
	gosync "sync"
	"testing"
	"time"
)

var testKey = gosync.OnceValue(func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
})

// keyJSON is a service account key file as the Cloud console issues it.
func keyJSON(t *testing.T, pemType string) string {
	t.Helper()
	var der []byte
	if pemType == "RSA PRIVATE KEY" {
		der = x509.MarshalPKCS1PrivateKey(testKey())
	} else {
		var err error
		if der, err = x509.MarshalPKCS8PrivateKey(testKey()); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  "acme-inventory",
		"private_key_id":              "3f2b1c0d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c",
		"private_key":                 string(pem.EncodeToMemory(&pem.Block{Type: pemType, Bytes: der})),
		"client_email":                "phoedox@acme-inventory.iam.gserviceaccount.com",
		"client_id":                   "104857600000000000001",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/phoedox%40acme-inventory.iam.gserviceaccount.com",
		"universe_domain":             "googleapis.com",
	})
	return string(b)
}

func TestAssertion(t *testing.T) {
	sa, err := parseServiceAccount(keyJSON(t, "PRIVATE KEY"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	jwt, err := sa.assertion("admin@acme.example", scopes, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatal(jwt)
	}
	dec := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var header map[string]string
	var claims map[string]any
	if json.Unmarshal(dec(parts[0]), &header) != nil || json.Unmarshal(dec(parts[1]), &claims) != nil {
		t.Fatal("not JSON")
	}
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "3f2b1c0d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c" {
		t.Fatal(header)
	}
	if claims["iss"] != "phoedox@acme-inventory.iam.gserviceaccount.com" || claims["sub"] != "admin@acme.example" || claims["aud"] != "https://oauth2.googleapis.com/token" ||
		claims["scope"] != "https://www.googleapis.com/auth/admin.directory.device.chromeos.readonly https://www.googleapis.com/auth/admin.directory.device.mobile.readonly" ||
		claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Unix()+3600) {
		t.Fatal(claims)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&testKey().PublicKey, crypto.SHA256, sum[:], dec(parts[2])); err != nil {
		t.Fatal("signature:", err)
	}
	v, err := url.ParseQuery(tokenBody(jwt))
	if err != nil || v.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || v.Get("assertion") != jwt {
		t.Fatal(v, err)
	}
}

func TestServiceAccount(t *testing.T) {
	if sa, err := parseServiceAccount(keyJSON(t, "RSA PRIVATE KEY")); err != nil || sa.Key.N.Cmp(testKey().N) != 0 {
		t.Fatal("PKCS #1:", err)
	}
	good := keyJSON(t, "PRIVATE KEY")
	var m map[string]string
	_ = json.Unmarshal([]byte(good), &m)
	with := func(k, v string) string {
		c := map[string]string{}
		for a, b := range m {
			c[a] = b
		}
		c[k] = v
		b, _ := json.Marshal(c)
		return string(b)
	}
	for name, raw := range map[string]string{
		"not json":     "-----BEGIN PRIVATE KEY-----",
		"user key":     with("type", "authorized_user"),
		"no email":     with("client_email", ""),
		"no key":       with("private_key", ""),
		"broken key":   with("private_key", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"),
		"not pem":      with("private_key", "MIIEvQIBADANBgkqhkiG9w0BAQEFAASC"),
		"certificate":  with("private_key", "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"),
		"wrong format": "{\"private_key\": 42}",
	} {
		_, err := parseServiceAccount(raw)
		if err == nil {
			t.Errorf("%s accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), "AAAA") || strings.Contains(err.Error(), "MIIE") {
			t.Errorf("%s: key material in error: %v", name, err)
		}
	}
	if validAdmin("admin@acme.example") != nil || validAdmin("admin") == nil || validAdmin("a b@c.d") == nil {
		t.Fatal("admin_email")
	}
}

func TestToken(t *testing.T) {
	if tok, err := parseToken(200, []byte(`{"access_token":"ya29.c.b0Aaek","expires_in":3599,"token_type":"Bearer"}`)); err != nil || tok != "ya29.c.b0Aaek" {
		t.Fatal(tok, err)
	}
	_, err := parseToken(401, []byte(`{"error":"unauthorized_client","error_description":"Client is unauthorized to retrieve access tokens using this method, or client not authorized for any of the scopes requested."}`))
	if err == nil || !strings.HasPrefix(err.Error(), "Google refused the sign-in (HTTP 401, unauthorized_client); allow") {
		t.Fatal(err)
	}
	_, err = parseToken(400, []byte(`{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`))
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "Signature") {
		t.Fatal(err)
	}
	if _, err := parseToken(502, []byte("<html>")); err == nil || err.Error() != "Google refused the sign-in (HTTP 502)" {
		t.Fatal(err)
	}
}
