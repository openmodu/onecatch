// Command onecatch-askpass is invoked by OpenSSH when a Remote FS workspace
// uses password authentication. It prints exactly one password retrieved from
// the operating system credential store. The password is never accepted in
// argv or the environment.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openmodu/onecatch/internal/sshcredentials"
)

func main() {
	prompt := ""
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}
	os.Exit(run(prompt, os.Stdout, sshcredentials.Lookup))
}

func run(prompt string, out io.Writer, lookup func(string) (string, error)) int {
	normalizedPrompt := strings.ToLower(strings.TrimSpace(prompt))
	if !strings.Contains(normalizedPrompt, "password") || strings.Contains(normalizedPrompt, "passphrase") {
		return 1
	}
	id := os.Getenv(sshcredentials.CredentialEnv)
	if id == "" {
		return 1
	}
	password, err := lookup(id)
	if err != nil || password == "" {
		return 1
	}
	if _, err := fmt.Fprintln(out, password); err != nil {
		return 1
	}
	return 0
}
