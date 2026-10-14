package sshendpoint

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  Endpoint
	}{
		{input: "devbox", want: Endpoint{Host: "devbox"}},
		{input: " 192.168.5.98:22 ", want: Endpoint{Host: "192.168.5.98", Port: 22}},
		{input: "::1", want: Endpoint{Host: "::1"}},
		{input: "[2001:db8::1]", want: Endpoint{Host: "2001:db8::1"}},
		{input: "[2001:db8::1]:2222", want: Endpoint{Host: "2001:db8::1", Port: 2222}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(test.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidPort(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"devbox:", "devbox:ssh", "devbox:0", "devbox:65536", "[::1]:bad"} {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", input)
		}
	}
}

func TestEndpointString(t *testing.T) {
	t.Parallel()
	if got := (Endpoint{Host: "2001:db8::1", Port: 2222}).String(); got != "[2001:db8::1]:2222" {
		t.Fatalf("String() = %q", got)
	}
}
