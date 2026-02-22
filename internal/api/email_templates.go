package api

import (
	"bytes"
	"embed"
	"fmt"
	htmltmpl "html/template"
	"os"
	"strings"
	"text/template"
	"time"
)

//go:embed email_templates/*.html email_templates/*.txt
var emailTemplateFS embed.FS

type emailBrand struct {
	AppName      string
	SupportEmail string
	LogoURL      string
	PrimaryColor string
	AccentColor  string
	CurrentYear  int
}

type verificationEmailData struct {
	Brand      emailBrand
	VerifyURL  string
	Preheader  string
	ExpiryText string
}

type resetCodeEmailData struct {
	Brand      emailBrand
	Code       string
	AppURL     string
	Preheader  string
	ExpiryText string
}

type genericEmailData struct {
	Brand     emailBrand
	Title     string
	Preheader string
	Message   string
	CTAURL    string
	CTALabel  string
}

type renderedEmail struct {
	Subject string
	Text    string
	HTML    string
}

func loadEmailBrand() emailBrand {
	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	if appName == "" {
		appName = "ExpenseLog"
	}
	supportEmail := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL"))
	if supportEmail == "" {
		supportEmail = "soporte@expenselog.com.ar"
	}
	logoURL := strings.TrimSpace(os.Getenv("APP_LOGO_URL"))
	if logoURL == "" {
		logoURL = "https://www.expenselog.com.ar/pwa/icon-192.png"
	}
	primary := strings.TrimSpace(os.Getenv("EMAIL_PRIMARY_COLOR"))
	if primary == "" {
		primary = "#2563EB"
	}
	accent := strings.TrimSpace(os.Getenv("EMAIL_ACCENT_COLOR"))
	if accent == "" {
		accent = "#0B1220"
	}
	return emailBrand{
		AppName:      appName,
		SupportEmail: supportEmail,
		LogoURL:      logoURL,
		PrimaryColor: primary,
		AccentColor:  accent,
		CurrentYear:  time.Now().Year(),
	}
}

func renderTemplatePair(baseName string, data any) (string, string, error) {
	htmlTpl, err := htmltmpl.ParseFS(emailTemplateFS, "email_templates/"+baseName+".html")
	if err != nil {
		return "", "", err
	}
	textTpl, err := template.ParseFS(emailTemplateFS, "email_templates/"+baseName+".txt")
	if err != nil {
		return "", "", err
	}
	var htmlBuf bytes.Buffer
	if err := htmlTpl.Execute(&htmlBuf, data); err != nil {
		return "", "", err
	}
	var textBuf bytes.Buffer
	if err := textTpl.Execute(&textBuf, data); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(textBuf.String()) + "\n", htmlBuf.String(), nil
}

func defaultAppURL() string {
	base := strings.TrimSpace(os.Getenv("APP_BASE_URL"))
	if base == "" {
		base = "https://www.expenselog.com.ar"
	}
	return strings.TrimRight(base, "/")
}

func buildVerificationEmail(verifyURL string) (renderedEmail, error) {
	brand := loadEmailBrand()
	data := verificationEmailData{
		Brand:      brand,
		VerifyURL:  verifyURL,
		Preheader:  "Confirma tu cuenta para empezar a registrar gastos e ingresos.",
		ExpiryText: "Este enlace expira en 24 horas.",
	}
	text, html, err := renderTemplatePair("verification", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{
		Subject: fmt.Sprintf("%s - Verifica tu email", brand.AppName),
		Text:    text,
		HTML:    html,
	}, nil
}

func buildResetCodeEmail(code, appURL string) (renderedEmail, error) {
	brand := loadEmailBrand()
	if strings.TrimSpace(appURL) == "" {
		appURL = defaultAppURL()
	}
	data := resetCodeEmailData{
		Brand:      brand,
		Code:       code,
		AppURL:     strings.TrimRight(appURL, "/"),
		Preheader:  "Usa este codigo para recuperar el acceso a tu cuenta.",
		ExpiryText: "El codigo vence en 15 minutos.",
	}
	text, html, err := renderTemplatePair("reset_code", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{
		Subject: fmt.Sprintf("%s - Codigo de recuperacion", brand.AppName),
		Text:    text,
		HTML:    html,
	}, nil
}

func buildPasswordChangedEmail(appURL string) (renderedEmail, error) {
	brand := loadEmailBrand()
	if strings.TrimSpace(appURL) == "" {
		appURL = defaultAppURL()
	}
	data := genericEmailData{
		Brand:     brand,
		Title:     "Tu contrasena fue actualizada",
		Preheader: "Confirmacion de seguridad de tu cuenta.",
		Message:   "Este email confirma que la contrasena de tu cuenta fue cambiada recientemente. Si no fuiste vos, te recomendamos recuperar el acceso de inmediato.",
		CTAURL:    strings.TrimRight(appURL, "/") + "/app/settings",
		CTALabel:  "Revisar seguridad",
	}
	text, html, err := renderTemplatePair("password_changed", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{
		Subject: fmt.Sprintf("%s - Contrasena actualizada", brand.AppName),
		Text:    text,
		HTML:    html,
	}, nil
}
