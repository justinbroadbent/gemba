// Package notify sends word of a decision to where the team talks.
//
// It is a stub for chat integration. Today it announces the few events the
// server itself performs: a trade-off, an expedite at triage, and a change of
// triage lead. Telling one person about a change to their own work would need
// the server to watch for changes, which it does not yet do; the browser does
// that part (see web/src/Toasts.tsx).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Event is one thing worth telling the team.
type Event struct {
	Workspace string // the workspace's name
	Title     string
	Text      string
}

type Notifier interface {
	Notify(ctx context.Context, e Event) error
}

// Nothing is the notifier for a workspace with no channel configured.
type Nothing struct{}

func (Nothing) Notify(context.Context, Event) error { return nil }

// Teams posts to a Microsoft Teams incoming webhook as an Adaptive Card.
// It has been tested against a local server only, not against Teams itself.
type Teams struct {
	Webhook string
	Client  *http.Client
}

func (t Teams) Notify(ctx context.Context, e Event) error {
	block := func(text string, bold bool) map[string]any {
		b := map[string]any{"type": "TextBlock", "text": text, "wrap": true}
		if bold {
			b["weight"] = "Bolder"
		}
		return b
	}
	body, _ := json.Marshal(map[string]any{
		"type": "message",
		"attachments": []any{map[string]any{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]any{
				"type": "AdaptiveCard", "version": "1.4",
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"body":    []any{block(e.Workspace+": "+e.Title, true), block(e.Text, false)},
			},
		}},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", t.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("teams: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("teams: HTTP %d", res.StatusCode)
	}
	return nil
}

// For builds the notifier for a webhook setting. A value of "env:NAME" reads
// the address from that environment variable, which keeps it out of the config
// file: anyone holding the address can post to the channel.
func For(teamsWebhook string) Notifier {
	if name, ok := strings.CutPrefix(teamsWebhook, "env:"); ok {
		teamsWebhook = os.Getenv(name)
	}
	if teamsWebhook == "" {
		return Nothing{}
	}
	return Teams{Webhook: teamsWebhook}
}
