package trakt

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"regexp"
	"testing"
)

func TestNewPKCE(t *testing.T) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`).MatchString(verifier) {
		t.Errorf("verifier %q is not a valid PKCE verifier", verifier)
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Errorf("challenge = %q, want S256 of verifier %q", challenge, want)
	}
	if v2, _, _ := NewPKCE(); v2 == verifier {
		t.Error("each sign-in needs a fresh verifier")
	}
}

func TestAuthorizeURL(t *testing.T) {
	u, err := url.Parse(NewClient("cid").AuthorizeURL("https://api.example/cb?x=1", "st4te", "chal"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "auth.trakt.tv" || u.Path != "/oauth/authorize" {
		t.Errorf("unexpected endpoint %s", u)
	}
	q := u.Query()
	want := map[string]string{
		"response_type": "code", "client_id": "cid", "redirect_uri": "https://api.example/cb?x=1",
		"state": "st4te", "code_challenge": "chal", "code_challenge_method": "S256", "scope": "public offline_access",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
}
