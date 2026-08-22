package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/openmodu/onecatch/internal/sshcredentials"
)

func TestRunPrintsPasswordFromCredentialStore(t *testing.T) {
	id, err := sshcredentials.NewID()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sshcredentials.CredentialEnv, id)
	var out bytes.Buffer
	code := run("deploy@devbox's password:", &out, func(got string) (string, error) {
		if got != id {
			t.Fatalf("credential id = %q, want %q", got, id)
		}
		return "correct horse battery staple", nil
	})
	if code != 0 || out.String() != "correct horse battery staple\n" {
		t.Fatalf("run = %d, output %q", code, out.String())
	}
}

func TestRunFailsClosedWithoutPrintingLookupErrors(t *testing.T) {
	id, err := sshcredentials.NewID()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sshcredentials.CredentialEnv, id)
	var out bytes.Buffer
	if code := run("Password for deploy@devbox:", &out, func(string) (string, error) { return "", errors.New("secret backend detail") }); code == 0 {
		t.Fatal("run unexpectedly succeeded")
	}
	if out.Len() != 0 {
		t.Fatalf("failure leaked output %q", out.String())
	}
}

func TestRunRefusesHostKeyAndPassphrasePrompts(t *testing.T) {
	id, err := sshcredentials.NewID()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sshcredentials.CredentialEnv, id)
	for _, prompt := range []string{
		"Are you sure you want to continue connecting (yes/no/[fingerprint])?",
		"Enter passphrase for key '/Users/me/.ssh/id_ed25519':",
	} {
		called := false
		if code := run(prompt, &bytes.Buffer{}, func(string) (string, error) { called = true; return "yes", nil }); code == 0 {
			t.Errorf("prompt %q was accepted", prompt)
		}
		if called {
			t.Errorf("prompt %q reached the credential store", prompt)
		}
	}
}
