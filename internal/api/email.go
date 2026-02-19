package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type smtpConfig struct {
	from     string
	fromName string
	host     string
	port     int
	user     string
	password string
}

type senderConfig struct {
	from     string
	fromName string
}

type brevoEmailPayload struct {
	Sender struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email"`
	} `json:"sender"`
	To      []map[string]string `json:"to"`
	Subject string              `json:"subject"`
	Text    string              `json:"textContent,omitempty"`
	HTML    string              `json:"htmlContent,omitempty"`
}

func loadSenderConfig() (senderConfig, error) {
	cfg := senderConfig{
		from:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		fromName: strings.TrimSpace(os.Getenv("SMTP_FROM_NAME")),
	}
	if cfg.fromName == "" {
		cfg.fromName = "ExpenseLog"
	}
	if cfg.from == "" {
		return senderConfig{}, fmt.Errorf("missing SMTP_FROM")
	}
	return cfg, nil
}

func loadSMTPConfig() (smtpConfig, error) {
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT")))
	if err != nil || port == 0 {
		return smtpConfig{}, fmt.Errorf("invalid SMTP_PORT")
	}
	sender, err := loadSenderConfig()
	if err != nil {
		return smtpConfig{}, err
	}
	cfg := smtpConfig{
		from:     sender.from,
		fromName: sender.fromName,
		host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		port:     port,
		user:     strings.TrimSpace(os.Getenv("SMTP_USER")),
		password: strings.TrimSpace(os.Getenv("SMTP_PASS")),
	}
	if cfg.host == "" || cfg.user == "" || cfg.password == "" {
		return smtpConfig{}, fmt.Errorf("missing SMTP config")
	}
	return cfg, nil
}

func loadBrevoConfig() (senderConfig, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("BREVO_API_KEY"))
	if apiKey == "" {
		return senderConfig{}, "", fmt.Errorf("missing BREVO_API_KEY")
	}
	sender, err := loadSenderConfig()
	if err != nil {
		return senderConfig{}, "", err
	}
	return sender, apiKey, nil
}

func sendBrevoEmail(toEmail, subject, textBody, htmlBody string) error {
	sender, apiKey, err := loadBrevoConfig()
	if err != nil {
		return err
	}
	var payload brevoEmailPayload
	payload.Sender.Email = sender.from
	if sender.fromName != "" {
		payload.Sender.Name = sender.fromName
	}
	payload.To = []map[string]string{{"email": toEmail}}
	payload.Subject = subject
	payload.Text = textBody
	payload.HTML = htmlBody

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("api-key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("brevo api error: status %d", resp.StatusCode)
	}
	return nil
}

func ensureHTMLBody(textBody, htmlBody string) string {
	htmlBody = strings.TrimSpace(htmlBody)
	if htmlBody != "" {
		return htmlBody
	}
	return "<pre style=\"font-family:Arial,sans-serif;white-space:pre-wrap;\">" + html.EscapeString(textBody) + "</pre>"
}

func sendSMTPEmail(toEmail, subject, textBody, htmlBody string) error {
	htmlBody = ensureHTMLBody(textBody, htmlBody)
	if strings.TrimSpace(os.Getenv("BREVO_API_KEY")) != "" {
		if err := sendBrevoEmail(toEmail, subject, textBody, htmlBody); err == nil {
			return nil
		}
	}
	cfg, err := loadSMTPConfig()
	if err != nil {
		return err
	}
	fromHeader := cfg.from
	if cfg.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", cfg.fromName, cfg.from)
	}
	boundary := "expenselog-boundary-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	msg := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + toEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		textBody,
		"",
		"--" + boundary,
		"Content-Type: text/html; charset=UTF-8",
		"",
		htmlBody,
		"",
		"--" + boundary + "--",
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", cfg.host, cfg.port)
	auth := smtp.PlainAuth("", cfg.user, cfg.password, cfg.host)

	if cfg.port == 465 {
		tlsCfg := &tls.Config{ServerName: cfg.host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.host)
		if err != nil {
			return err
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return err
		}
		if err := client.Mail(cfg.from); err != nil {
			return err
		}
		if err := client.Rcpt(toEmail); err != nil {
			return err
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write([]byte(msg)); err != nil {
			return err
		}
		return writer.Close()
	}

	return smtp.SendMail(addr, auth, cfg.from, []string{toEmail}, []byte(msg))
}

func sendResetCodeEmail(toEmail, code, appURL string) error {
	email, err := buildResetCodeEmail(code, appURL)
	if err != nil {
		return err
	}
	return sendSMTPEmail(toEmail, email.Subject, email.Text, email.HTML)
}

func sendVerificationEmail(toEmail, verifyURL string) error {
	email, err := buildVerificationEmail(verifyURL)
	if err != nil {
		return err
	}
	return sendSMTPEmail(toEmail, email.Subject, email.Text, email.HTML)
}

func sendPasswordChangedEmail(toEmail, appURL string) error {
	email, err := buildPasswordChangedEmail(appURL)
	if err != nil {
		return err
	}
	return sendSMTPEmail(toEmail, email.Subject, email.Text, email.HTML)
}
