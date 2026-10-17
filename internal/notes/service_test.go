package notes

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.dalw.in/holocron/internal/server"
)

func mk(t *testing.T) *NotesService {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTwoDevicesConverge(t *testing.T) {
	st, _ := server.OpenStore(filepath.Join(t.TempDir(), "j"))
	ts := httptest.NewServer(server.Handler(st, "tok"))
	defer ts.Close()
	a, b := mk(t), mk(t)
	for _, d := range []*NotesService{a, b} {
		must(t, d.SetServer(ts.URL, "tok"))
	}
	n, err := a.Create("Work/Ideas", "Plan")
	must(t, err)
	_, err = a.Save(n.ID, "# Plan\n\nv1", []string{"Launch", "#q3"})
	must(t, err)
	a.SyncNow()
	if s := b.SyncNow(); s.LastError != "" {
		t.Fatal(s.LastError)
	}
	got, err := b.Get(n.ID)
	must(t, err)
	if got.Path != "Work/Ideas/Plan" || got.Content != "# Plan\n\nv1" || strings.Join(got.Tags, ",") != "launch,q3" {
		t.Fatalf("B got %+v", got)
	}
	// concurrent edits + a move on B
	_, err = a.Save(n.ID, "from A", got.Tags)
	must(t, err)
	_, err = b.Save(n.ID, "from B", got.Tags)
	must(t, err)
	a.SyncNow()
	b.SyncNow() // conflict -> B keeps its text as a copy
	b.SyncNow() // pushes the copy
	a.SyncNow()
	for name, d := range map[string]*NotesService{"A": a, "B": b} {
		tr, _ := d.Tree()
		if len(tr.Notes) != 2 {
			t.Fatalf("%s has %d notes: %+v", name, len(tr.Notes), tr.Notes)
		}
		var texts []string
		for _, m := range tr.Notes {
			g, _ := d.Get(m.ID)
			texts = append(texts, g.Content)
		}
		if strings.Join(texts, "|") != "from A|from B" && strings.Join(texts, "|") != "from B|from A" {
			t.Fatalf("%s texts: %v", name, texts)
		}
	}
	// delete on A propagates to B
	must(t, a.Delete(n.ID))
	a.SyncNow()
	b.SyncNow()
	if _, err := b.Get(n.ID); err == nil {
		t.Fatal("delete did not propagate")
	}
	if s := a.Status(); s.Pending != 0 {
		t.Fatalf("pending %d", s.Pending)
	}
}

func TestImportPreservesFrontMatterAndRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "notes", "Inbox"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes", "Inbox", "a.md"), []byte("---\ntitle: Hello\n---\nbody"), 0o644)
	s, err := New(dir)
	must(t, err)
	tr, _ := s.Tree()
	if len(tr.Notes) != 1 || tr.Notes[0].Path != "Inbox/a" {
		t.Fatalf("%+v", tr)
	}
	_, err = s.Save(tr.Notes[0].ID, "edited", []string{"x"})
	must(t, err)
	raw, _ := os.ReadFile(filepath.Join(dir, "notes", "Inbox", "a.md"))
	if !strings.Contains(string(raw), "title: Hello") || !strings.HasSuffix(string(raw), "---\nedited") {
		t.Fatalf("front matter lost: %q", raw)
	}
	if _, err := s.Move(tr.Notes[0].ID, "../../etc/x"); err == nil {
		t.Fatal("traversal allowed")
	}
}
