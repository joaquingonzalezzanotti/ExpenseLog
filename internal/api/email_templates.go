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

type RenderedEmail struct {
	Subject string
	Text    string
	HTML    string
}

type renderedEmail = RenderedEmail

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
		Subject: fmt.Sprintf("%s · Verifica tu email", brand.AppName),
		Text:    text,
		HTML:    html,
	}, nil
}

func buildResetCodeEmail(code string) (renderedEmail, error) {
	brand := loadEmailBrand()
	data := resetCodeEmailData{
		Brand:      brand,
		Code:       code,
		Preheader:  "Usa este código para recuperar el acceso a tu cuenta.",
		ExpiryText: "El código vence en 15 minutos.",
	}
	text, html, err := renderTemplatePair("reset_code", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{
		Subject: fmt.Sprintf("%s · Código de recuperación", brand.AppName),
		Text:    text,
		HTML:    html,
	}, nil
}

func buildWelcomeEmail() (renderedEmail, error) {
	brand := loadEmailBrand()
	data := genericEmailData{
		Brand:     brand,
		Title:     "¡Bienvenido/a a ExpenseLog!",
		Preheader: "Tu cuenta está lista para organizar tus finanzas.",
		Message:   "Tu cuenta ya está activa. Te recomendamos configurar moneda base, categorías y empezar con tu primer movimiento.",
		CTAURL:    "https://www.expenselog.com.ar/app",
		CTALabel:  "Abrir panel",
	}
	text, html, err := renderTemplatePair("welcome", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{Subject: fmt.Sprintf("%s · Tu cuenta está lista", brand.AppName), Text: text, HTML: html}, nil
}

func buildPasswordChangedEmail() (renderedEmail, error) {
	brand := loadEmailBrand()
	data := genericEmailData{
		Brand:     brand,
		Title:     "Tu contraseña fue actualizada",
		Preheader: "Confirmación de seguridad de tu cuenta.",
		Message:   "Este email confirma que la contraseña de tu cuenta fue cambiada recientemente. Si no fuiste vos, te recomendamos recuperar el acceso de inmediato.",
		CTAURL:    "https://www.expenselog.com.ar/app",
		CTALabel:  "Ir a ExpenseLog",
	}
	text, html, err := renderTemplatePair("password_changed", data)
	if err != nil {
		return renderedEmail{}, err
	}
	return renderedEmail{Subject: fmt.Sprintf("%s · Contraseña actualizada", brand.AppName), Text: text, HTML: html}, nil
}

func BuildEmailPreviews(baseURL string) (map[string]RenderedEmail, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://www.expenselog.com.ar"
	}
	base = strings.TrimRight(base, "/")

	verification, err := buildVerificationEmail(base + "/auth/verify?token=preview-token")
	if err != nil {
		return nil, err
	}
	resetCode, err := buildResetCodeEmail("123456")
	if err != nil {
		return nil, err
	}
	welcome, err := buildWelcomeEmail()
	if err != nil {
		return nil, err
	}
	passwordChanged, err := buildPasswordChangedEmail()
	if err != nil {
		return nil, err
	}

	return map[string]RenderedEmail{
		"verification":     verification,
		"reset_code":       resetCode,
		"welcome":          welcome,
		"password_changed": passwordChanged,
	}, nil
}
