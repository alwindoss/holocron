package syncer

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.dalw.in/holocron/internal/proto"
	"go.dalw.in/holocron/internal/server"
)

type memLocal struct {
	notes  map[string]proto.Note
	base   map[string]int64
	dirty  map[string]bool
	cursor int64
	n      int
}

func newMem() *memLocal {
	return &memLocal{notes: map[string]proto.Note{}, base: map[string]int64{}, dirty: map[string]bool{}}
}
func (m *memLocal) edit(id, path, content string) {
	m.notes[id] = proto.Note{ID: id, Path: path, Content: content}
	m.dirty[id] = true
}
func (m *memLocal) Dirty() (out []Pending, _ error) {
	for id := range m.dirty {
		out = append(out, Pending{m.notes[id], m.base[id]})
	}
	return
}
func (m *memLocal) IsDirty(id string) bool { return m.dirty[id] }
func (m *memLocal) Apply(n proto.Note) error {
	m.notes[n.ID], m.base[n.ID] = n, n.Rev
	delete(m.dirty, n.ID)
	return nil
}
func (m *memLocal) MarkPushed(n proto.Note) error { return m.Apply(n) }
func (m *memLocal) SaveConflictCopy(l proto.Note) error {
	m.n++
	m.edit(l.ID+"-c"+string(rune('0'+m.n)), l.Path, l.Content)
	return nil
}
func (m *memLocal) Cursor() int64           { return m.cursor }
func (m *memLocal) SetCursor(c int64) error { m.cursor = c; return nil }

func TestConflictNeverLosesText(t *testing.T) {
	st, err := server.OpenStore(filepath.Join(t.TempDir(), "j"))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler(st, "tok"))
	defer ts.Close()
	ctx := context.Background()
	a, b := newMem(), newMem()
	ca := &Client{BaseURL: ts.URL, Token: "tok", Local: a}
	cb := &Client{BaseURL: ts.URL, Token: "tok", Local: b}

	a.edit("n1", "Work/Plan", "v1")
	must(t, ca.Sync(ctx))
	must(t, cb.Sync(ctx)) // B now has v1
	a.edit("n1", "Work/Plan", "from A")
	b.edit("n1", "Work/Plan", "from B") // concurrent edit, same base rev
	must(t, ca.Sync(ctx))
	must(t, cb.Sync(ctx)) // conflict: B takes A's text, keeps its own as a copy
	if b.notes["n1"].Content != "from A" {
		t.Fatalf("original should hold server text, got %q", b.notes["n1"].Content)
	}
	must(t, cb.Sync(ctx)) // pushes the conflict copy
	must(t, ca.Sync(ctx)) // A pulls it
	found := false
	for _, n := range a.notes {
		if n.Content == "from B" && n.Path == "Work/Plan (conflict)" {
			found = true
		}
	}
	if !found {
		t.Fatal("B's edit was lost")
	}
}

func TestRetryIsIdempotentAndTornJournalRecovers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "j")
	st, _ := server.OpenStore(p)
	n := proto.Note{ID: "x", Path: "P", Content: "hi", Hash: proto.Hash("hi")}
	r1, err := st.Push(proto.PushRequest{Note: n})
	must(t, err)
	r2, err := st.Push(proto.PushRequest{Note: n}) // lost-response retry
	must(t, err)
	if r1.Rev != 1 || r2.Rev != 1 {
		t.Fatalf("retry created a new revision: %d %d", r1.Rev, r2.Rev)
	}
	st.Close()
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("deadbeef {\"id\":\"y\",\"pa") // simulated crash mid-write
	f.Close()
	st2, err := server.OpenStore(p)
	must(t, err)
	if c := st2.Changes(0); len(c.Notes) != 1 || c.Notes[0].ID != "x" {
		t.Fatalf("bad recovery: %+v", c)
	}
	if _, err := st2.Push(proto.PushRequest{Note: proto.Note{ID: "z", Content: "", Hash: proto.Hash("")}}); err != nil {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
