package services

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/sentinel-app/sentinel/internal/config"
)

// EmailService sends emails via AWS SES.
type EmailService struct {
	client *ses.Client
	cfg    *config.Config
}

// NewEmailService creates a new SES-backed email service.
func NewEmailService(cfg *config.Config) *EmailService {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithSharedConfigProfile(cfg.AWSProfile),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	return &EmailService{
		client: ses.NewFromConfig(awsCfg),
		cfg:    cfg,
	}
}

// SendOTP sends an OTP verification email.
func (s *EmailService) SendOTP(ctx context.Context, to, subject, htmlBody string) error {
	return s.send(ctx, s.cfg.SESFromOTP, to, subject, htmlBody)
}

// SendAlert sends an alert email when a timer expires.
func (s *EmailService) SendAlert(ctx context.Context, to, subject, htmlBody string) error {
	return s.send(ctx, s.cfg.SESFromAlert, to, subject, htmlBody)
}

// SendAlertWithAttachments sends an alert email with raw MIME (for attachments).
// This uses SendRawEmail to support file attachments.
func (s *EmailService) SendAlertRaw(ctx context.Context, rawMessage []byte, from string, recipients []string) error {
	input := &ses.SendRawEmailInput{
		Source:       aws.String(from),
		Destinations: recipients,
		RawMessage: &types.RawMessage{
			Data: rawMessage,
		},
	}

	_, err := s.client.SendRawEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to send raw email: %w", err)
	}
	return nil
}

func (s *EmailService) send(ctx context.Context, from, to, subject, htmlBody string) error {
	input := &ses.SendEmailInput{
		Source: aws.String(from),
		Destination: &types.Destination{
			ToAddresses: []string{to},
		},
		Message: &types.Message{
			Subject: &types.Content{
				Data:    aws.String(subject),
				Charset: aws.String("UTF-8"),
			},
			Body: &types.Body{
				Html: &types.Content{
					Data:    aws.String(htmlBody),
					Charset: aws.String("UTF-8"),
				},
			},
		},
	}

	_, err := s.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to send email via SES: %w", err)
	}
	return nil
}
