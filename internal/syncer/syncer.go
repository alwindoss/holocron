// Package syncer is the client-side sync engine. It talks to the server and
// to local storage through the Local interface, so the Wails app (files on
// disk) and tests (in memory) share the same logic.
package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"go.dalw.in/holocron/internal/proto"
)

// Pending is a locally edited note plus the server revision it was based on.
type Pending struct {
	Note    proto.Note
	BaseRev int64
}

// Local is implemented by the app's note store. Every method that writes
// must be durable and atomic (use atomicfile.Write for the .md files).
type Local interface {
	Dirty() ([]Pending, error)               // notes with unsynced local edits
	IsDirty(id string) bool                  // has unsynced local edits?
	Apply(remote proto.Note) error           // write server version, record its rev, clear dirty
	MarkPushed(n proto.Note) error           // record server rev after a successful push
	SaveConflictCopy(local proto.Note) error // store local edit as a NEW note (new ID, dirty)
	Cursor() int64
	SetCursor(int64) error
}

type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
	Local          Local
}

// Sync pushes local edits, then pulls remote changes. Safe to re-run at any
// point: every step is idempotent and no step discards data.
func (c *Client) Sync(ctx context.Context) error {
	pending, err := c.Local.Dirty()
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := c.push(ctx, p); err != nil {
			return err
		}
	}
	return c.pull(ctx)
}

func (c *Client) push(ctx context.Context, p Pending) error {
	p.Note.Hash = proto.Hash(p.Note.Content)
	code, srv, err := c.do(ctx, "PUT", "/v1/notes/"+url.PathEscape(p.Note.ID), proto.PushRequest{BaseRev: p.BaseRev, Note: p.Note})
	if err != nil {
		return err
	}
	switch code {
	case 200:
		return c.Local.MarkPushed(srv)
	case 409:
		if srv.Hash == p.Note.Hash && srv.Path == p.Note.Path && srv.Deleted == p.Note.Deleted {
			return c.Local.Apply(srv) // both sides made the same edit
		}
		if !p.Note.Deleted { // keep OUR text as a separate note, then take theirs
			p.Note.Path += " (conflict)"
			if err := c.Local.SaveConflictCopy(p.Note); err != nil {
				return err
			}
		}
		return c.Local.Apply(srv)
	}
	return fmt.Errorf("push %s: HTTP %d", p.Note.ID, code)
}

func (c *Client) pull(ctx context.Context) error {
	var ch proto.Changes
	code, _, err := c.doInto(ctx, "GET", fmt.Sprintf("/v1/changes?since=%d", c.Local.Cursor()), nil, &ch)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("pull: HTTP %d", code)
	}
	for _, n := range ch.Notes {
		if c.Local.IsDirty(n.ID) {
			continue // resolved on the next push via the 409 path
		}
		if err := c.Local.Apply(n); err != nil {
			return err
		}
	}
	return c.Local.SetCursor(ch.Cursor) // only after everything is applied
}

func (c *Client) do(ctx context.Context, method, path string, body any) (int, proto.Note, error) {
	var n proto.Note
	code, _, err := c.doInto(ctx, method, path, body, &n)
	return code, n, err
}

func (c *Client) doInto(ctx context.Context, method, path string, body, out any) (int, any, error) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 409 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return 0, nil, err
		}
	}
	return resp.StatusCode, nil, nil
}
