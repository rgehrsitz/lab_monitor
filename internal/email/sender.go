package email

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
)

type Sender struct {
	client        *ses.Client
	sender        string
	subjectPrefix string
	dryRun        bool
}

func NewSender(ctx context.Context, sender, subjectPrefix string, dryRun bool) (*Sender, error) {
	if sender == "" {
		return nil, errors.New("sender email address required")
	}

	// In dry-run mode, we don't need AWS credentials
	if dryRun {
		return &Sender{
			client:        nil,
			sender:        sender,
			subjectPrefix: subjectPrefix,
			dryRun:        true,
		}, nil
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID_SES")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY_SES")
	if accessKey == "" || secretKey == "" {
		return nil, errors.New("AWS_ACCESS_KEY_ID_SES and AWS_SECRET_ACCESS_KEY_SES must be set")
	}

	region := os.Getenv("AWS_REGION_SES")
	if region == "" {
		region = "us-east-2" // default region
	}

	creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &Sender{
		client:        ses.NewFromConfig(cfg),
		sender:        sender,
		subjectPrefix: subjectPrefix,
		dryRun:        false,
	}, nil
}

func (s *Sender) Send(ctx context.Context, recipients []string, subject, textBody, htmlBody string) error {
	if len(recipients) == 0 {
		return errors.New("no recipients provided")
	}

	subjectLine := subject
	if s.subjectPrefix != "" {
		subjectLine = fmt.Sprintf("%s %s", s.subjectPrefix, subject)
	}

	if s.dryRun {
		fmt.Printf("\n=== DRY RUN: Email would be sent ===\n")
		fmt.Printf("From: %s\n", s.sender)
		fmt.Printf("To: %v\n", recipients)
		fmt.Printf("Subject: %s\n", subjectLine)
		fmt.Printf("\n--- Text Body ---\n%s\n", textBody)
		if htmlBody != "" {
			fmt.Printf("\n--- HTML Body ---\n%s\n", htmlBody)
		}
		fmt.Printf("=====================================\n\n")
		return nil
	}

	body := &types.Body{}
	if htmlBody != "" {
		body.Html = &types.Content{Data: aws.String(htmlBody), Charset: aws.String(awsUTF8)}
	}
	if textBody != "" {
		body.Text = &types.Content{Data: aws.String(textBody), Charset: aws.String(awsUTF8)}
	}
	input := &ses.SendEmailInput{
		Destination: &types.Destination{ToAddresses: recipients},
		Message: &types.Message{
			Body:    body,
			Subject: &types.Content{Data: aws.String(subjectLine), Charset: aws.String(awsUTF8)},
		},
		Source: aws.String(s.sender),
	}

	_, err := s.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

const awsUTF8 = "UTF-8"
