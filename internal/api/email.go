package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type smtpConfig struct {
	host     string
	port     int
	user     string
	password string
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
	Text    string              `json:"textContent"`
}

func loadSMTPConfig() (smtpConfig, error) {
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT")))
	if err != nil || port == 0 {
		return smtpConfig{}, fmt.Errorf("invalid SMTP_PORT")
	}
	cfg := smtpConfig{
		host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		port:     port,
		user:     strings.TrimSpace(os.Getenv("SMTP_USER")),
		password: strings.TrimSpace(os.Getenv("SMTP_PASS")),
		from:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		fromName: strings.TrimSpace(os.Getenv("SMTP_FROM_NAME")),
	}
	if cfg.fromName == "" {
		cfg.fromName = "ExpenseLog"
	}
	if cfg.host == "" || cfg.user == "" || cfg.password == "" || cfg.from == "" {
		return smtpConfig{}, fmt.Errorf("missing SMTP config")
	}
	return cfg, nil
}

func loadBrevoConfig() (smtpConfig, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("BREVO_API_KEY"))
	if apiKey == "" {
		return smtpConfig{}, "", fmt.Errorf("missing BREVO_API_KEY")
	}
	cfg, err := loadSMTPConfig()
	if err != nil {
		return smtpConfig{}, "", err
	}
	return cfg, apiKey, nil
}

func sendBrevoEmail(toEmail, subject, body string) error {
	cfg, apiKey, err := loadBrevoConfig()
	if err != nil {
		return err
	}
	var payload brevoEmailPayload
	payload.Sender.Email = cfg.from
	if cfg.fromName != "" {
		payload.Sender.Name = cfg.fromName
	}
	payload.To = []map[string]string{{"email": toEmail}}
	payload.Subject = subject
	payload.Text = body

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

func sendSMTPEmail(toEmail, subject, body string) error {
	if strings.TrimSpace(os.Getenv("BREVO_API_KEY")) != "" {
		if err := sendBrevoEmail(toEmail, subject, body); err == nil {
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
	msg := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + toEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
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

func sendResetCodeEmail(toEmail, code string) error {
	subject := "ExpenseLog - Codigo de recuperacion"
	body := fmt.Sprintf("Hola,\n\nTu codigo de recuperacion de ExpenseLog es: %s\n\nEste codigo expira en 15 minutos.\nSi no pediste este codigo, podes ignorar este mensaje.\n\nGracias,\nEquipo ExpenseLog\n", code)
	return sendSMTPEmail(toEmail, subject, body)
}

func sendVerificationEmail(toEmail, verifyURL string) error {
	subject := "ExpenseLog - Verifica tu email"
	body := fmt.Sprintf("Hola,\n\nPara verificar tu cuenta de ExpenseLog, hace click en este enlace:\n%s\n\nEste enlace expira en 24 horas.\nSi no pediste esta cuenta, podes ignorar este mensaje.\n\nGracias,\nEquipo ExpenseLog\n", verifyURL)
	return sendSMTPEmail(toEmail, subject, body)
}
