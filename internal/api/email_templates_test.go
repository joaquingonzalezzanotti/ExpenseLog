package api

import (
	"strings"
	"testing"
)

func TestBuildVerificationEmail(t *testing.T) {
	verifyURL := "https://www.expenselog.com.ar/api/auth/verify?token=abc"
	email, err := buildVerificationEmail(verifyURL)
	if err != nil {
		t.Fatalf("buildVerificationEmail returned error: %v", err)
	}
	if !strings.Contains(email.Subject, "Verifica tu email") {
		t.Fatalf("unexpected subject: %q", email.Subject)
	}
	if !strings.Contains(email.Text, verifyURL) {
		t.Fatalf("verification url missing in plain text body")
	}
	if !strings.Contains(email.HTML, verifyURL) {
		t.Fatalf("verification url missing in html body")
	}
}

func TestBuildResetCodeEmail(t *testing.T) {
	appURL := "https://www.expenselog.com.ar"
	email, err := buildResetCodeEmail("123456", appURL)
	if err != nil {
		t.Fatalf("buildResetCodeEmail returned error: %v", err)
	}
	if !strings.Contains(email.Subject, "Codigo de recuperacion") {
		t.Fatalf("unexpected subject: %q", email.Subject)
	}
	if !strings.Contains(email.Text, "123456") {
		t.Fatalf("reset code missing in plain text body")
	}
	if !strings.Contains(email.HTML, ">123456<") {
		t.Fatalf("reset code missing in html body")
	}
	if !strings.Contains(email.Text, appURL+"/app") {
		t.Fatalf("app URL missing in reset text body")
	}
}

func TestBuildPasswordChangedEmail(t *testing.T) {
	appURL := "https://www.expenselog.com.ar"
	email, err := buildPasswordChangedEmail(appURL)
	if err != nil {
		t.Fatalf("buildPasswordChangedEmail returned error: %v", err)
	}
	if !strings.Contains(email.Subject, "Contrasena actualizada") {
		t.Fatalf("unexpected subject: %q", email.Subject)
	}
	if !strings.Contains(email.Text, appURL+"/app/settings") {
		t.Fatalf("settings URL missing in plain text body")
	}
	if !strings.Contains(email.HTML, appURL+"/app/settings") {
		t.Fatalf("settings URL missing in html body")
	}
}
