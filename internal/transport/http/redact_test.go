package httptransport

import (
	"strconv"
	"strings"
	"testing"
)

func TestRedactToken(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"abc":               "****",
		"abcd":              "****",
		"secret-token-1234": "****1234",
	}
	for in, want := range cases {
		if got := redactToken(in); got != want {
			t.Fatalf("redactToken(%q) = %q, want %q", in, got, want)
		}
	}
	// A redacted token must never contain the secret middle.
	if strings.Contains(redactToken("supersecretvalue"), "supersecret") {
		t.Fatal("redactToken leaked the secret body")
	}
}

func TestPromptDigestHidesContent(t *testing.T) {
	prompt := "请帮我完成 2026 年中国 AI Agent 服务市场研究。"
	digest := promptDigest(prompt)
	if strings.Contains(digest, "市场研究") || strings.Contains(digest, "Agent") {
		t.Fatalf("promptDigest leaked content: %q", digest)
	}
	if !strings.Contains(digest, strconv.Itoa(len([]rune(prompt)))) {
		t.Fatalf("promptDigest %q missing rune count", digest)
	}
}
