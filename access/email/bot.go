package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"gopkg.in/mail.v2"

	"github.com/mailgun/mailgun-go/v4"
)

var reviewReplyTemplate = template.Must(template.New("review reply").Parse(
	`{{.Author}} reviewed the request at {{.Created.Format .TimeFormat}}.
Resolution: {{.ProposedStateEmoji}} {{.ProposedState}}.
{{if .Reason}}Reason: {{.Reason}}.{{end}}`,
))

// Bot is a email client that works with access.Request. Essentially it's not a bot, but let's
// keep naming for the sake of uniformity.
type Bot struct {
	clusterName string
	sender      string
	mg          *mailgun.MailgunImpl
	smtp        *mail.Dialer
	webProxyURL *url.URL
}

// NewBot initializes the new Email message generator (Bot)
// takes EmailConfig as an argument.
func NewBot(ctx context.Context, conf Config, clusterName, webProxyAddr string) (Bot, error) {
	var (
		webProxyURL *url.URL
		err         error
		mg          *mailgun.MailgunImpl
		smtp        *mail.Dialer
	)

	if webProxyAddr != "" {
		if webProxyURL, err = lib.AddrToURL(webProxyAddr); err != nil {
			return Bot{}, trace.Wrap(err)
		}
	}

	if conf.Mailgun != nil {
		mg = mailgun.NewMailgun(conf.Mailgun.Domain, conf.Mailgun.PrivateKey)
		logger.Get(ctx).WithField("domain", conf.Mailgun.Domain).Info("Using Mailgun as email transport")
	}

	if conf.SMTP != nil {
		smtp = mail.NewDialer(conf.SMTP.Host, conf.SMTP.Port, conf.SMTP.Username, conf.SMTP.Password)
		smtp.StartTLSPolicy = mail.MandatoryStartTLS
	}

	return Bot{
		mg:          mg,
		smtp:        smtp,
		sender:      conf.Delivery.Sender,
		clusterName: clusterName,
		webProxyURL: webProxyURL,
	}, nil
}

// SendNewThreads sends emails on new requests. Returns EmailData.
func (b *Bot) SendNewThreads(ctx context.Context, recipients []string, reqID string, reqData RequestData) ([]EmailThread, error) {
	var threads []EmailThread
	var errors []error

	body := b.buildBody(reqID, reqData, "You have a new Role Request")

	for _, email := range recipients {
		id, err := b.send(reqID, email, body, "")
		if err != nil {
			errors = append(errors, err)
			continue
		}

		threads = append(threads, EmailThread{Email: email, Timestamp: time.Now().String(), MessageID: id})
	}

	return threads, trace.NewAggregate(errors...)
}

// SendReviews sends message new AccessReview message to the given threads
func (b Bot) SendReview(ctx context.Context, threads []EmailThread, reqID string, reqData RequestData, review types.AccessReview) ([]EmailThread, error) {
	var proposedStateEmoji string
	var threadsSent = make([]EmailThread, 0)

	switch review.ProposedState {
	case types.RequestState_APPROVED:
		proposedStateEmoji = "✅"
	case types.RequestState_DENIED:
		proposedStateEmoji = "❌"
	}

	var builder strings.Builder
	err := reviewReplyTemplate.Execute(&builder, struct {
		types.AccessReview
		ProposedState      string
		ProposedStateEmoji string
		TimeFormat         string
	}{
		review,
		review.ProposedState.String(),
		proposedStateEmoji,
		time.RFC822,
	})
	if err != nil {
		return threadsSent, trace.Wrap(err)
	}
	body := builder.String()

	errors := make([]error, 0)

	for _, thread := range threads {
		_, err = b.send(reqID, thread.Email, body, thread.MessageID)
		if err != nil {
			errors = append(errors, trace.Wrap(err))
			continue
		}
		threadsSent = append(threadsSent, thread)
	}

	return threadsSent, trace.NewAggregate(errors...)
}

// SendResolveStatus sends message on a request status update (review, decline)
func (b Bot) SendResolveStatus(ctx context.Context, threads []EmailThread, reqID string, reqData RequestData) ([]EmailThread, error) {
	var errors []error
	var threadsSent = make([]EmailThread, 0)

	body := b.buildBody(reqID, reqData, "Role Request has been resolved")

	for _, thread := range threads {
		_, err := b.send(reqID, thread.Email, body, thread.MessageID)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		threadsSent = append(threads, thread)
	}

	return threadsSent, trace.NewAggregate(errors...)
}

// buildBody builds a email message for create/resolve events
func (b Bot) buildBody(reqID string, reqData RequestData, subject string) string {
	var builder strings.Builder
	builder.Grow(128)

	builder.WriteString(fmt.Sprintf("%v:\n\n", subject))

	resolution := reqData.Resolution

	msgFieldToBuilder(&builder, "ID", reqID)
	msgFieldToBuilder(&builder, "Cluster", b.clusterName)

	if len(reqData.User) > 0 {
		msgFieldToBuilder(&builder, "User", reqData.User)
	}
	if reqData.Roles != nil {
		msgFieldToBuilder(&builder, "Role(s)", strings.Join(reqData.Roles, ","))
	}
	if reqData.RequestReason != "" {
		msgFieldToBuilder(&builder, "Reason", reqData.RequestReason)
	}
	if b.webProxyURL != nil {
		reqURL := *b.webProxyURL
		reqURL.Path = lib.BuildURLPath("web", "requests", reqID)
		msgFieldToBuilder(&builder, "Link", reqURL.String())
	} else {
		if resolution.Tag == Unresolved {
			msgFieldToBuilder(&builder, "Approve", fmt.Sprintf("tsh request review --approve %s", reqID))
			msgFieldToBuilder(&builder, "Deny", fmt.Sprintf("tsh request review --deny %s", reqID))
		}
	}

	var statusEmoji string
	status := string(resolution.Tag)
	switch resolution.Tag {
	case Unresolved:
		status = "PENDING"
		statusEmoji = "⏳"
	case ResolvedApproved:
		statusEmoji = "✅"
	case ResolvedDenied:
		statusEmoji = "❌"
	case ResolvedExpired:
		statusEmoji = "⌛"
	}

	statusText := fmt.Sprintf("Status: %s %s", statusEmoji, status)
	if resolution.Reason != "" {
		statusText += fmt.Sprintf(" (%s)", resolution.Reason)
	}

	builder.WriteString("\n")
	builder.WriteString(statusText)

	return builder.String()
}

// msgFieldToBuilder utility string builder method
func msgFieldToBuilder(b *strings.Builder, field, value string) {
	b.WriteString(field)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}

// send sends email with given request id, body and references value to recipient
func (b *Bot) send(id, recipient, body, references string) (string, error) {
	subject := fmt.Sprintf("%v Role Request %v", b.clusterName, id)
	refHeader := fmt.Sprintf("<%v>", references)

	if b.mg != nil {
		m := b.mg.NewMessage(b.sender, subject, body, recipient)
		m.SetRequireTLS(true)

		if references != "" {
			m.AddHeader("References", references)
			m.AddHeader("In-Reply-To", references)
		}

		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
		defer cancel()

		_, id, err := b.mg.Send(ctx, m)

		if err != nil {
			return "", trace.Wrap(err)
		}

		return id, nil
	}

	if b.smtp != nil {
		id, err := genMessageID()
		if err != nil {
			return "", trace.Wrap(err)
		}

		m := mail.NewMessage()

		m.SetHeader("From", b.sender)
		m.SetHeader("To", recipient)
		m.SetHeader("Subject", subject)
		m.SetHeader("Message-ID", fmt.Sprintf("<%v>", id))
		m.SetBody("text/plain", body)

		if references != "" {
			m.SetHeader("References", refHeader)
			m.SetHeader("In-Reply-To", refHeader)
		}

		err = b.smtp.DialAndSend(m)
		if err != nil {
			return "", trace.Wrap(err)
		}

		return id, nil
	}

	// This won't ever happen
	return "", trace.BadParameter("No active transports")
}

// genMessageID generates Message-ID header value
func genMessageID() (string, error) {
	now := uint64(time.Now().UnixNano())

	nonceByte := make([]byte, 8)
	if _, err := rand.Read(nonceByte); err != nil {
		return "", trace.Wrap(err)
	}
	nonce := binary.BigEndian.Uint64(nonceByte)

	hostname, err := os.Hostname()
	if err != nil {
		return "", trace.Wrap(err)
	}

	msgID := fmt.Sprintf("%s.%s@%s", base36(now), base36(nonce), hostname)

	return msgID, nil
}

// base36 converts given value to a base 36 numbering system
func base36(input uint64) string {
	return strings.ToUpper(strconv.FormatUint(input, 36))
}
