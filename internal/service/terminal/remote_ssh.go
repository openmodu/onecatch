package terminal

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/openmodu/onecatch/internal/sshcredentials"
	"github.com/openmodu/onecatch/internal/sshendpoint"
)

// remoteSSHInvocation builds the interactive SSH process hosted by the local
// PTY. The workspace password stays in the system keyring and is supplied by
// the same askpass helper used by Remote FS commands and SFTP.
func remoteSSHInvocation(input CreateInput, environment []string) (string, []string, []string, string, error) {
	target := input.RemoteFS
	if target == nil {
		return "", nil, nil, "", fmt.Errorf("remote terminal target is missing")
	}
	endpoint, err := sshendpoint.Parse(target.Host)
	if err != nil {
		return "", nil, nil, "", err
	}
	batchMode := "yes"
	if target.CredentialID != "" {
		batchMode = "no"
	}
	args := []string{
		"-o", "BatchMode=" + batchMode,
		"-o", "ClearAllForwardings=yes",
		"-o", "ForwardAgent=no",
		"-o", "SendEnv=-*",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-tt",
	}
	if endpoint.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", endpoint.Port))
	}
	if target.CredentialID != "" {
		args = append(args,
			"-o", "PubkeyAuthentication=no",
			"-o", "KbdInteractiveAuthentication=no",
			"-o", "PreferredAuthentications=password",
			"-o", "NumberOfPasswordPrompts=1",
		)
	}
	for _, option := range target.SSHOptions {
		if strings.TrimSpace(option) != "" {
			args = append(args, "-o", option)
		}
	}
	if username := strings.TrimSpace(target.Username); username != "" {
		args = append(args, "-l", username)
	}
	args = append(args, endpoint.Host, "cd "+quoteRemoteShellWord(target.Root)+` && exec "${SHELL:-/bin/sh}" -l`)

	binary := "ssh"
	configured := exec.Command(binary)
	configured.Env = environment
	if err := sshcredentials.ConfigureCommand(configured, target.CredentialID, ""); err != nil {
		return "", nil, nil, "", fmt.Errorf("configure SSH authentication: %w", err)
	}
	label := endpoint.String()
	if username := strings.TrimSpace(target.Username); username != "" {
		label = username + "@" + label
	}
	return binary, args, configured.Env, label, nil
}

func quoteRemoteShellWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
