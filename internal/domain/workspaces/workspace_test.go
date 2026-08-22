package workspaces

import "testing"

func TestValidateRemoteFSWorkspace(t *testing.T) {
	valid := Workspace{
		ID: "remote-project", Name: "Remote project", Path: "/srv/project",
		RemoteFS: &RemoteFS{Host: "devbox", Root: "/srv/project"},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid remote workspace: %v", err)
	}
	invalid := valid
	invalid.RemoteFS = &RemoteFS{Host: "", Root: "/srv/project"}
	if err := Validate(invalid); err == nil {
		t.Fatal("remote workspace without a host was accepted")
	}
	invalid = valid
	invalid.RemoteFS = &RemoteFS{Host: "devbox:not-a-port", Root: "/srv/project"}
	if err := Validate(invalid); err == nil {
		t.Fatal("remote workspace with an invalid port was accepted")
	}
	invalid = valid
	invalid.RemoteFS = &RemoteFS{Host: "devbox", Root: "/srv/other"}
	if err := Validate(invalid); err == nil {
		t.Fatal("workspace path differing from remote root was accepted")
	}
	invalid = valid
	invalid.RemoteFS = &RemoteFS{Host: "devbox", Root: "/srv/project", CredentialID: "sshcred_0123456789abcdef0123456789abcdef"}
	if err := Validate(invalid); err == nil {
		t.Fatal("password credential without a username was accepted")
	}
}
