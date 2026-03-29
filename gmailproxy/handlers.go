package gmailproxy

import (
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"
)

type Proxy struct {
	svc     *gmail.Service
	labelID string
}

func NewProxy(svc *gmail.Service, labelID string) *Proxy {
	return &Proxy{svc: svc, labelID: labelID}
}

func (p *Proxy) ListMessages(maxResults int64, pageToken, query string) (*gmail.ListMessagesResponse, error) {
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 20
	}

	call := p.svc.Users.Messages.List("me").LabelIds(p.labelID).MaxResults(maxResults)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	if query != "" {
		call = call.Q(query)
	}

	return call.Do()
}

func (p *Proxy) GetMessage(id, format string) (*gmail.Message, error) {
	if id == "" {
		return nil, fmt.Errorf("missing message id")
	}
	if format == "" {
		format = "full"
	}

	msg, err := p.svc.Users.Messages.Get("me", id).Format(format).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail get error: %w", err)
	}

	if !HasLabel(msg, p.labelID) {
		return nil, fmt.Errorf("message not accessible")
	}

	return msg, nil
}

func (p *Proxy) GetAttachment(msgID, attID string) (*gmail.MessagePartBody, error) {
	if msgID == "" || attID == "" {
		return nil, fmt.Errorf("missing message or attachment id")
	}

	msg, err := p.svc.Users.Messages.Get("me", msgID).Format("minimal").Do()
	if err != nil {
		return nil, fmt.Errorf("gmail get error: %w", err)
	}
	if !HasLabel(msg, p.labelID) {
		return nil, fmt.Errorf("message not accessible")
	}

	return p.svc.Users.Messages.Attachments.Get("me", msgID, attID).Do()
}

type DraftRequest struct {
	To         []string `json:"to"`
	CC         []string `json:"cc,omitempty"`
	BCC        []string `json:"bcc,omitempty"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
	HTMLBody   string   `json:"htmlBody,omitempty"`
	InReplyTo  string   `json:"inReplyTo,omitempty"`
	References string   `json:"references,omitempty"`
	ThreadId   string   `json:"threadId,omitempty"`
}

type DraftResult struct {
	ID        string `json:"id"`
	MessageID string `json:"messageId"`
}

func (p *Proxy) CreateDraft(req DraftRequest) (*DraftResult, error) {
	if len(req.To) == 0 {
		return nil, fmt.Errorf("at least one 'to' recipient required")
	}

	raw := buildRFC2822(req)
	encoded := base64.URLEncoding.EncodeToString([]byte(raw))

	draft := &gmail.Draft{
		Message: &gmail.Message{
			Raw:      encoded,
			ThreadId: req.ThreadId,
		},
	}

	created, err := p.svc.Users.Drafts.Create("me", draft).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail draft create error: %w", err)
	}

	return &DraftResult{
		ID:        created.Id,
		MessageID: created.Message.Id,
	}, nil
}


// sanitizeHeader strips \r and \n to prevent header injection.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

const mimeBoundary = "----=_Part_gmail_proxy"

func buildRFC2822(req DraftRequest) string {
	var b strings.Builder
	sanitized := make([]string, len(req.To))
	for i, v := range req.To {
		sanitized[i] = sanitizeHeader(v)
	}
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(sanitized, ", "))
	if len(req.CC) > 0 {
		sanitized = make([]string, len(req.CC))
		for i, v := range req.CC {
			sanitized[i] = sanitizeHeader(v)
		}
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(sanitized, ", "))
	}
	if len(req.BCC) > 0 {
		sanitized = make([]string, len(req.BCC))
		for i, v := range req.BCC {
			sanitized[i] = sanitizeHeader(v)
		}
		fmt.Fprintf(&b, "Bcc: %s\r\n", strings.Join(sanitized, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(req.Subject))
	if req.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", sanitizeHeader(req.InReplyTo))
	}
	if req.References != "" {
		fmt.Fprintf(&b, "References: %s\r\n", sanitizeHeader(req.References))
	}
	b.WriteString("MIME-Version: 1.0\r\n")

	if req.HTMLBody == "" {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(req.Body)
	} else {
		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n", mimeBoundary)
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "--%s\r\n", mimeBoundary)
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(req.Body)
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "--%s\r\n", mimeBoundary)
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(req.HTMLBody)
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "--%s--\r\n", mimeBoundary)
	}

	return b.String()
}
