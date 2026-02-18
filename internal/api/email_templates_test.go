package api

import (
	"strings"
	"testing"
)

func TestBuildVerificationEmail(t *testing.T) {
	email, err := buildVerificationEmail("https://www.expenselog.com.ar/auth/verify?token=abc")
	if err != nil {
		t.Fatalf("buildVerificationEmail returned error: %v", err)
	}
	if !strings.Contains(email.Subject, "Verifica tu email") {
		t.Fatalf("unexpected subject: %q", email.Subject)
	}
	if !strings.Contains(email.Text, "https://www.expenselog.com.ar/auth/verify?token=abc") {
		t.Fatalf("verification url missing in plain text body")
	}
	if !strings.Contains(email.HTML, "Verificar email") {
		t.Fatalf("CTA button label missing in html body")
	}
}

func TestBuildResetCodeEmail(t *testing.T) {
	email, err := buildResetCodeEmail("123456")
	if err != nil {
		t.Fatalf("buildResetCodeEmail returned error: %v", err)
	}
	if !strings.Contains(email.Subject, "Código de recuperación") {
		t.Fatalf("unexpected subject: %q", email.Subject)
	}
	if !strings.Contains(email.Text, "123456") {
		t.Fatalf("reset code missing in plain text body")
	}
	if !strings.Contains(email.HTML, ">123456<") {
		t.Fatalf("reset code missing in html body")
	}
}

func TestAdditionalTemplatesRender(t *testing.T) {
	if _, err := buildWelcomeEmail(); err != nil {
		t.Fatalf("buildWelcomeEmail returned error: %v", err)
	}
	if _, err := buildPasswordChangedEmail(); err != nil {
		t.Fatalf("buildPasswordChangedEmail returned error: %v", err)
	}
}
