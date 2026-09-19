package core

import (
	"errors"
	"strings"
	"testing"
)

func TestRemoteAccessMessages(t *testing.T) {
	tests := []struct {
		name   string
		err    string
		remote string
		want   string
	}{
		{"unauthorized", "unable to access remote https://github.com/acme/team.git: authentication required", "https://github.com/acme/team.git", "dossier signin"},
		{"forbidden", "unable to access remote: authorization failed (403)", "https://github.com/acme/team.git", "Your GitHub account can't see acme/team"},
		{"not found", "remote returned 404", "https://github.com/acme/team.git", "Your GitHub account can't see acme/team"},
		{"repository not found", "repository not found", "https://github.com/acme/team.git", "Your GitHub account can't see acme/team"},
		{"sso", "remote returned SAML SSO authorization required", "https://github.com/acme/team.git", "Authorize the GitHub CLI for your organization"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remoteAccessMessage(tt.remote, errors.New(tt.err))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("remoteAccessMessage() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestAuthFailedMessageUsesSignin(t *testing.T) {
	got := authFailedMessage("https://github.com/acme/team.git")
	if !strings.Contains(got, "run `dossier signin`") || !strings.Contains(got, "~/.dossier/credentials") {
		t.Fatalf("authFailedMessage() = %q", got)
	}
}
