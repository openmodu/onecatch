package workflows

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseOutcomeMessagePreservesReportAndRawJSON(t *testing.T) {
	raw := `{"verdict":"pass","summary":"已完成：保留 {参数} 和 \"引号\"。","extra":{"count":12,"code":"\n` + "```" + `sh\n测试\n` + "```" + `"}}`
	report := "## 核对\n\n- 测试通过\n- 工作树干净\n\n```sh\ngit diff --check\n```\n\n未执行部署。"
	for _, test := range []struct{ name, text, prose string }{
		{"bare", raw, ""},
		{"json-fence", "```json\n" + raw + "\n```", ""},
		{"plain-fence", "```\n" + raw + "\n```", ""},
		{"crlf-fence", "```json\r\n" + raw + "\r\n```", ""},
		{"report-and-json", report + "\n\n" + raw, report},
		{"report-and-json-fence", report + "\n\n```json\n" + raw + "\n```", report},
		{"report-and-plain-fence", report + "\n\n```\n" + raw + "\n```", report},
		{"leading-code-report", "```sh\ngit status\n```\n\n" + raw, "```sh\ngit status\n```"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, err := ParseOutcomeMessage(" \n" + test.text + "\n ")
			if err != nil {
				t.Fatal(err)
			}
			if message.Prose != test.prose || message.JSON != raw || message.Verdict != VerdictPass || message.Summary != `已完成：保留 {参数} 和 "引号"。` {
				t.Fatalf("parsed message = %+v", message)
			}
			out, err := ParseOutcome(test.text)
			if err != nil || !reflect.DeepEqual(out, message.Outcome) {
				t.Fatalf("orchestration/presentation disagreement: %+v, %v", out, err)
			}
		})
	}
}

func TestParseOutcomeMessageRejectsExamplesAndPartialResponses(t *testing.T) {
	raw := `{"verdict":"pass","summary":"完成"}`
	for _, text := range []string{
		"报告。\n" + raw + "\n后面还有正文。",
		"报告。\n```json\n" + raw,
		"报告。\n```go\n" + raw + "\n```",
		"报告。\n~~~json\n" + raw,
		"报告。\n" + strings.TrimSuffix(raw, "}"),
		"报告。\n```json\n" + raw + "\n```\n这是示例。",
		`{"verdict":"pass","summary":""}`,
		`{"verdict":"approve","summary":"完成"}`,
	} {
		if _, err := ParseOutcomeMessage(text); !errors.Is(err, ErrProtocol) {
			t.Fatalf("ParseOutcomeMessage(%q) error = %v", text, err)
		}
	}
}

func TestParseOutcomeMessageNormalizesFallbackFeedback(t *testing.T) {
	message, err := ParseOutcomeMessage(`{"verdict":" rework ","target":" implement ","feedback":"补测试"}`)
	if err != nil || message.Verdict != VerdictRework || message.Target != "implement" || message.Summary != "补测试" || message.Feedback != "补测试" {
		t.Fatalf("feedback-only result = %+v, %v", message, err)
	}
}
