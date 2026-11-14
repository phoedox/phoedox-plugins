// Service account sign-in for Google APIs (OAuth 2.0 JWT bearer grant,
// RFC 7523), without build tags so it is testable natively. Errors never
// contain the key or the assertion.
package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	tokenHost = "oauth2.googleapis.com"
	tokenURL  = "https://" + tokenHost + "/token"
	scopes    = "https://www.googleapis.com/auth/admin.directory.device.chromeos.readonly https://www.googleapis.com/auth/admin.directory.device.mobile.readonly"
	jwtBearer = "urn:ietf:params:oauth:grant-type:jwt-bearer"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ServiceAccount is the part of a service account JSON key that is used.
type ServiceAccount struct {
	ClientEmail  string
	PrivateKeyID string
	Key          *rsa.PrivateKey
}

// parseServiceAccount reads a service account JSON key.
func parseServiceAccount(raw string) (*ServiceAccount, error) {
	var k struct {
		Type         string `json:"type"`
		ClientEmail  string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		PrivateKeyID string `json:"private_key_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &k); err != nil {
		return nil, errors.New("the setting service_account_json must be the JSON key file of a service account")
	}
	if k.Type != "" && k.Type != "service_account" {
		return nil, fmt.Errorf("the setting service_account_json is a %q key, not a service account key", k.Type)
	}
	if !emailRe.MatchString(k.ClientEmail) || k.PrivateKey == "" {
		return nil, errors.New("the setting service_account_json has no client_email or private_key")
	}
	key, err := parsePrivateKey(k.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &ServiceAccount{ClientEmail: k.ClientEmail, PrivateKeyID: k.PrivateKeyID, Key: key}, nil
}

// parsePrivateKey reads an RSA key in PEM (PKCS #8 as Google issues it, or
// PKCS #1).
func parsePrivateKey(s string) (*rsa.PrivateKey, error) {
	bad := errors.New("the private_key of the service account is not a PEM-encoded RSA key")
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, bad
	}
	switch block.Type {
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, bad
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, bad
		}
		return rk, nil
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, bad
		}
		return k, nil
	}
	return nil, bad
}

// validAdmin checks the admin_email setting.
func validAdmin(s string) error {
	if !emailRe.MatchString(s) {
		return errors.New("the setting admin_email must be the e-mail address of a Google Workspace administrator")
	}
	return nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// assertion builds the signed JWT (RS256) that asks for an access token on
// behalf of subject. It is valid for one hour from now.
func (sa *ServiceAccount) assertion(subject, scope string, now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if sa.PrivateKeyID != "" {
		header["kid"] = sa.PrivateKeyID
	}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"sub":   subject,
		"scope": scope,
		"aud":   tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64(h) + "." + b64(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(nil, sa.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("signing with the service account key failed")
	}
	return signing + "." + b64(sig), nil
}

// tokenBody is the JWT bearer grant request.
func tokenBody(assertion string) string {
	return url.Values{"grant_type": {jwtBearer}, "assertion": {assertion}}.Encode()
}

// parseToken reads the access token of a token answer. Errors name only the
// error code and a hint.
func parseToken(status int, body []byte) (string, error) {
	var r struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	if status == 200 && r.AccessToken != "" {
		return r.AccessToken, nil
	}
	msg := fmt.Sprintf("Google refused the sign-in (HTTP %d", status)
	if r.Error != "" && len(r.Error) < 64 {
		msg += ", " + r.Error
	}
	msg += ")"
	switch r.Error {
	case "unauthorized_client":
		msg += "; allow the service account's client ID both device scopes under domain-wide delegation in the Admin console"
	case "invalid_grant":
		msg += "; check admin_email, that the key is still active and the server clock"
	}
	return "", errors.New(msg)
}
