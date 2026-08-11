package fetch

import (
	"strings"
	"testing"
)

func TestNormalizeTargetURLRejectsUnsafeTargets(t *testing.T) {
	cases := []string{
		"",
		"not a url",
		"ftp://example.com",
		"http://user:pass@example.com",
		"http://localhost",
		"http://foo.local",
		"http://foo.internal",
		"http://foo.localhost",
		"http://127.0.0.1",
		"http://[::1]",
		"http://10.0.0.1",
		"http://169.254.169.254/latest/meta-data/",
		"http://192.168.1.1",
		"http://[fe80::1]",
		"http://example.com:9200",
	}
	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := NormalizeTargetURL(rawURL); err == nil {
				t.Fatalf("expected rejection for %q", rawURL)
			}
		})
	}
}

func TestNormalizeTargetURLCanonicalizesPublicHTTPS(t *testing.T) {
	normalized, err := NormalizeTargetURL("  https://EXAMPLE.com/article?ref=one#section  ")
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if normalized != "https://example.com/article?ref=one" {
		t.Fatalf("normalized URL = %q", normalized)
	}
}

func TestNormalizeTargetURLRejectedMessagesAreActionable(t *testing.T) {
	_, err := NormalizeTargetURL("http://127.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error message should explain why: %v", err)
	}
}
