package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamsPostsACard(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type        string
			Attachments []struct {
				ContentType string
				Content     struct{ Body []struct{ Text string } }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Type != "message" || len(body.Attachments) != 1 {
			t.Errorf("unexpected payload: %v %+v", err, body)
			return
		}
		for _, b := range body.Attachments[0].Content.Body {
			got += b.Text + "\n"
		}
	}))
	defer srv.Close()

	t.Setenv("HOOK", srv.URL)
	n := For("env:HOOK")
	if err := n.Notify(context.Background(), Event{Workspace: "Infra", Title: "Trade-off recorded", Text: "A displaces B."}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Infra: Trade-off recorded") || !strings.Contains(got, "A displaces B.") {
		t.Errorf("the card should carry the title and text, got %q", got)
	}
	if _, quiet := For("").(Nothing); !quiet {
		t.Error("no webhook means nothing is sent")
	}
	if _, quiet := For("env:NOT_SET_ANYWHERE").(Nothing); !quiet {
		t.Error("an unset variable means nothing is sent")
	}
}
