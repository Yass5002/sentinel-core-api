package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
)

// OTPData holds the data for the OTP verification email template.
type OTPData struct {
	Code          string
	ExpiryMinutes int
}

// AttachmentInfo holds display metadata for a file attachment.
type AttachmentInfo struct {
	Filename string
	Size     string // human-readable, e.g. "2.4 MB"
	MimeType string
}

// AlertData holds the data for the dead man's switch alert email template.
type AlertData struct {
	UserName     string
	UserEmail    string
	Message      string
	Location     template.HTML // pre-sanitised HTML, won't be escaped
	EventCreated string
	TimerExpired string
	Attachments  []AttachmentInfo
}

// Engine loads and renders HTML templates from disk.
type Engine struct {
	otp   *template.Template
	alert *template.Template
}

// NewEngine parses the HTML templates in dir and returns a ready-to-use Engine.
// dir should point to the folder containing otp.html and alert.html.
func NewEngine(dir string) (*Engine, error) {
	otp, err := template.ParseFiles(filepath.Join(dir, "otp.html"))
	if err != nil {
		return nil, fmt.Errorf("parse otp.html: %w", err)
	}
	alert, err := template.ParseFiles(filepath.Join(dir, "alert.html"))
	if err != nil {
		return nil, fmt.Errorf("parse alert.html: %w", err)
	}
	return &Engine{otp: otp, alert: alert}, nil
}

// RenderOTP renders the OTP verification email to an HTML string.
func (e *Engine) RenderOTP(data OTPData) (string, error) {
	var buf bytes.Buffer
	if err := e.otp.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderAlert renders the dead man's switch alert email to an HTML string.
func (e *Engine) RenderAlert(data AlertData) (string, error) {
	var buf bytes.Buffer
	if err := e.alert.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
