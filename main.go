package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stephen/gmail-proxy/gmailproxy"
)

func main() {
	tokenJSON := requireTokenJSON()
	allowedLabel := requireEnv("ALLOWED_LABEL")
	authToken := requireEnv("MCP_AUTH_TOKEN")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()

	gmailSvc, err := gmailproxy.NewGmailService(ctx, tokenJSON)
	if err != nil {
		log.Fatalf("gmail init error: %v", err)
	}
	log.Println("gmail service initialized")

	labelID, err := gmailproxy.ResolveLabelID(gmailSvc, allowedLabel)
	if err != nil {
		log.Fatalf("label resolution error: %v", err)
	}
	log.Printf("resolved label %q to ID %q", allowedLabel, labelID)

	proxy := gmailproxy.NewProxy(gmailSvc, labelID)

	mcpServer := server.NewMCPServer(
		"gmail-proxy",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	registerTools(mcpServer, proxy)

	httpServer := server.NewStreamableHTTPServer(mcpServer, server.WithStateLess(true))

	mux := http.NewServeMux()
	mux.Handle("/mcp", bearerAuth(authToken, httpServer))

	addr := ":" + port
	log.Printf("starting gmail MCP server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func registerTools(s *server.MCPServer, proxy *gmailproxy.Proxy) {
	s.AddTool(mcp.NewTool("list_messages",
		mcp.WithDescription("List emails that have the allowed label. Returns message IDs and thread IDs; use get_message to fetch full content."),
		mcp.WithNumber("maxResults", mcp.Description("Max messages to return (1-100, default 20)")),
		mcp.WithString("pageToken", mcp.Description("Page token for pagination")),
		mcp.WithString("q", mcp.Description("Gmail search query to further filter results")),
		mcp.WithReadOnlyHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		maxResults := int64(req.GetInt("maxResults", 20))
		pageToken := req.GetString("pageToken", "")
		query := req.GetString("q", "")

		resp, err := proxy.ListMessages(maxResults, pageToken, query)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return toJSONResult(resp)
	})

	s.AddTool(mcp.NewTool("get_message",
		mcp.WithDescription("Get a single email by ID. Only accessible if the message has the allowed label."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Message ID")),
		mcp.WithString("format", mcp.Description("Response format: full, metadata, minimal, raw (default: full)"),
			mcp.Enum("full", "metadata", "minimal", "raw")),
		mcp.WithReadOnlyHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := req.GetString("id", "")
		format := req.GetString("format", "full")

		msg, err := proxy.GetMessage(id, format)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return toJSONResult(msg)
	})

	s.AddTool(mcp.NewTool("get_attachment",
		mcp.WithDescription("Get an email attachment. The parent message must have the allowed label."),
		mcp.WithString("messageId", mcp.Required(), mcp.Description("Parent message ID")),
		mcp.WithString("attachmentId", mcp.Required(), mcp.Description("Attachment ID")),
		mcp.WithReadOnlyHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		msgID := req.GetString("messageId", "")
		attID := req.GetString("attachmentId", "")

		att, err := proxy.GetAttachment(msgID, attID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return toJSONResult(att)
	})

	s.AddTool(mcp.NewTool("create_draft",
		mcp.WithDescription("Create an email draft. Does NOT send the email."),
		mcp.WithArray("to", mcp.Required(), mcp.Description("Recipient email addresses"), mcp.WithStringItems()),
		mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Email body (plain text). Also used as the fallback for clients that don't render HTML.")),
		mcp.WithString("htmlBody", mcp.Description("Optional HTML email body. When provided, the email is sent as multipart/alternative with both plain text and HTML parts.")),
		mcp.WithArray("cc", mcp.Description("CC recipients"), mcp.WithStringItems()),
		mcp.WithArray("bcc", mcp.Description("BCC recipients"), mcp.WithStringItems()),
		mcp.WithString("inReplyTo", mcp.Description("Message-ID header of the email being replied to")),
		mcp.WithString("references", mcp.Description("References header for email threading")),
		mcp.WithString("threadId", mcp.Description("Gmail thread ID to attach the draft to")),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dr := gmailproxy.DraftRequest{
			To:         req.GetStringSlice("to", nil),
			CC:         req.GetStringSlice("cc", nil),
			BCC:        req.GetStringSlice("bcc", nil),
			Subject:    req.GetString("subject", ""),
			Body:       req.GetString("body", ""),
			HTMLBody:   req.GetString("htmlBody", ""),
			InReplyTo:  req.GetString("inReplyTo", ""),
			References: req.GetString("references", ""),
			ThreadId:   req.GetString("threadId", ""),
		}

		result, err := proxy.CreateDraft(dr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return toJSONResult(result)
	})

}

func bearerAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func toJSONResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}

func requireTokenJSON() string {
	if v := strings.TrimSpace(os.Getenv("GMAIL_TOKEN_JSON")); v != "" {
		return v
	}

	path := strings.TrimSpace(os.Getenv("GMAIL_TOKEN_FILE"))
	if path == "" {
		log.Fatal("one of GMAIL_TOKEN_JSON or GMAIL_TOKEN_FILE is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("reading GMAIL_TOKEN_FILE %q: %v", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		log.Fatalf("GMAIL_TOKEN_FILE %q is empty", path)
	}
	return string(data)
}
