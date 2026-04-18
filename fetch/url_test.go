package fetch

import (
	"context"
	"strings"
	"testing"
)

func TestValidateTargetURL_RejectsUnsafeTargets(t *testing.T) {
	cases := []string{
		"",
		"not a url",
		"ftp://example.com",
		"http://user:pass@example.com",
		"http://localhost",
		"http://foo.local",
		"http://foo.internal",
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
			if err := ValidateTargetURL(context.Background(), rawURL); err == nil {
				t.Fatalf("expected rejection for %q", rawURL)
			}
		})
	}
}

func TestValidateTargetURL_AllowsPublicHTTPS(t *testing.T) {
	// 93.184.216.34 was example.com's public IP; using a literal avoids
	// needing DNS in the test environment.
	if err := ValidateTargetURL(context.Background(), "https://93.184.216.34"); err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
}

func TestValidateTargetURL_RejectedMessagesAreActionable(t *testing.T) {
	err := ValidateTargetURL(context.Background(), "http://127.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error message should explain why: %v", err)
	}
}
