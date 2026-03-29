package gmailproxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/api/gmail/v1"
)

type Proxy struct {
	svc      *gmail.Service
	labelID  string
}

func NewProxy(svc *gmail.Service, labelID string) *Proxy {
	return &Proxy{svc: svc, labelID: labelID}
}

func (p *Proxy) ListMessages(w http.ResponseWriter, r *http.Request) {
	maxResults := int64(20)
	if v := r.URL.Query().Get("maxResults"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 && n <= 100 {
			maxResults = n
		}
	}

	call := p.svc.Users.Messages.List("me").LabelIds(p.labelID).MaxResults(maxResults)

	if pt := r.URL.Query().Get("pageToken"); pt != "" {
		call = call.PageToken(pt)
	}
	if q := r.URL.Query().Get("q"); q != "" {
		call = call.Q(q)
	}

	resp, err := call.Do()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gmail list error: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (p *Proxy) GetMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing message id")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "full"
	}

	msg, err := p.svc.Users.Messages.Get("me", id).Format(format).Do()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gmail get error: %v", err)
		return
	}

	if !HasLabel(msg, p.labelID) {
		writeError(w, http.StatusForbidden, "message not accessible")
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (p *Proxy) GetAttachment(w http.ResponseWriter, r *http.Request) {
	msgID := r.PathValue("id")
	attID := r.PathValue("attachmentId")
	if msgID == "" || attID == "" {
		writeError(w, http.StatusBadRequest, "missing message or attachment id")
		return
	}

	// Verify parent message has the allowed label.
	msg, err := p.svc.Users.Messages.Get("me", msgID).Format("minimal").Do()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gmail get error: %v", err)
		return
	}
	if !HasLabel(msg, p.labelID) {
		writeError(w, http.StatusForbidden, "message not accessible")
		return
	}

	att, err := p.svc.Users.Messages.Attachments.Get("me", msgID, attID).Do()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gmail attachment error: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, att)
}

type DraftRequest struct {
	To         []string `json:"to"`
	CC         []string `json:"cc,omitempty"`
	BCC        []string `json:"bcc,omitempty"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
	InReplyTo  string   `json:"inReplyTo,omitempty"`
	References string   `json:"references,omitempty"`
	ThreadId   string   `json:"threadId,omitempty"`
}

func (p *Proxy) CreateDraft(w http.ResponseWriter, r *http.Request) {
	var req DraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if len(req.To) == 0 {
		writeError(w, http.StatusBadRequest, "at least one 'to' recipient required")
		return
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
		writeError(w, http.StatusBadGateway, "gmail draft create error: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":        created.Id,
		"messageId": created.Message.Id,
	})
}

func (p *Proxy) ListLabels(w http.ResponseWriter, r *http.Request) {
	resp, err := p.svc.Users.Labels.List("me").Do()
	if err != nil {
		writeError(w, http.StatusBadGateway, "gmail labels error: %v", err)
		return
	}

	type label struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	out := make([]label, len(resp.Labels))
	for i, l := range resp.Labels {
		out[i] = label{ID: l.Id, Name: l.Name, Type: l.Type}
	}

	writeJSON(w, http.StatusOK, out)
}

func buildRFC2822(req DraftRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(req.To, ", "))
	if len(req.CC) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(req.CC, ", "))
	}
	if len(req.BCC) > 0 {
		fmt.Fprintf(&b, "Bcc: %s\r\n", strings.Join(req.BCC, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", req.Subject)
	if req.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", req.InReplyTo)
	}
	if req.References != "" {
		fmt.Fprintf(&b, "References: %s\r\n", req.References)
	}
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(req.Body)
	return b.String()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("error: %s", msg)
	writeJSON(w, status, map[string]string{"error": msg})
}
