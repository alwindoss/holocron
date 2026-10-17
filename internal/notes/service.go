// Package notes is the Wails service: markdown files in folders on disk,
// with id + tags in YAML front matter, plus sync state under .holocron/.
package notes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.dalw.in/holocron/internal/atomicfile"
	"go.dalw.in/holocron/internal/proto"
	"go.dalw.in/holocron/internal/syncer"
)

type NoteMeta struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Tags    []string `json:"tags"`
	Updated int64    `json:"updated"` // unix ms
}
type Note struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Tags    []string `json:"tags"`
	Content string   `json:"content"`
}
type Tree struct {
	Notes   []NoteMeta `json:"notes"`
	Folders []string   `json:"folders"`
}
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
type ServerSettings struct {
	URL      string `json:"url"`
	HasToken bool   `json:"hasToken"`
}
type SyncStatus struct {
	Configured bool   `json:"configured"`
	Pending    int    `json:"pending"`
	LastSync   int64  `json:"lastSync"`
	LastError  string `json:"lastError"`
}

type entry struct {
	Path    string
	Tags    []string
	Extra   string
	Updated int64
}
type ns struct {
	BaseRev int64 `json:"baseRev"`
	Dirty   bool  `json:"dirty"`
	Deleted bool  `json:"deleted"`
}
type state struct {
	ServerURL string         `json:"serverUrl"`
	Token     string         `json:"token"`
	Cursor    int64          `json:"cursor"`
	Notes     map[string]*ns `json:"notes"`
}

var errNotFound = errors.New("note not found")

type NotesService struct {
	mu      sync.Mutex
	syncing sync.Mutex
	root    string
	meta    string
	idx     map[string]*entry
	st      state
	last    int64
	lastErr string
}

// New opens (or creates) a notes library under dataDir.
func New(dataDir string) (*NotesService, error) {
	s := &NotesService{root: filepath.Join(dataDir, "notes"), meta: filepath.Join(dataDir, ".holocron"), idx: map[string]*entry{}}
	s.st.Notes = map[string]*ns{}
	for _, d := range []string{s.root, s.meta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if b, err := os.ReadFile(filepath.Join(s.meta, "state.json")); err == nil {
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, err
		}
		if s.st.Notes == nil {
			s.st.Notes = map[string]*ns{}
		}
	}
	if err := s.scan(); err != nil {
		return nil, err
	}
	return s, s.saveState()
}

func (s *NotesService) file(rel string) string {
	return filepath.Join(s.root, filepath.FromSlash(rel)+".md")
}
func (s *NotesService) saveState() error {
	b, _ := json.Marshal(s.st)
	return atomicfile.Write(filepath.Join(s.meta, "state.json"), b)
}
func (s *NotesService) touch(id string) {
	n := s.st.Notes[id]
	if n == nil {
		n = &ns{}
		s.st.Notes[id] = n
	}
	n.Dirty, n.Deleted = true, false
}

// scan indexes .md files; files without an id (dropped in from elsewhere)
// get one, and files removed outside the app become tombstones.
func (s *NotesService) scan() error {
	seen := map[string]bool{}
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != s.root {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		id, tags, extra, body := parse(string(raw))
		if id == "" || seen[id] {
			id = newID()
			if err := atomicfile.Write(p, []byte(render(id, tags, extra, body))); err != nil {
				return err
			}
			s.touch(id)
		}
		seen[id] = true
		rel, _ := filepath.Rel(s.root, p)
		info, _ := d.Info()
		s.idx[id] = &entry{strings.TrimSuffix(filepath.ToSlash(rel), ".md"), tags, extra, info.ModTime().UnixMilli()}
		if s.st.Notes[id] == nil {
			s.touch(id)
		}
		return nil
	})
	for id, n := range s.st.Notes {
		if !seen[id] && !n.Deleted {
			if n.BaseRev == 0 {
				delete(s.st.Notes, id)
			} else {
				n.Deleted, n.Dirty = true, true
			}
		}
	}
	return err
}

func (s *NotesService) unique(rel, self string) string {
	cand := rel
	for i := 2; ; i++ {
		if e := s.idx[self]; e != nil && e.Path == cand {
			return cand
		}
		if _, err := os.Stat(s.file(cand)); errors.Is(err, fs.ErrNotExist) {
			return cand
		}
		cand = fmt.Sprintf("%s (%d)", rel, i)
	}
}

func (s *NotesService) write(id string, e *entry, body string) error {
	if err := atomicfile.Write(s.file(e.Path), []byte(render(id, e.Tags, e.Extra, body))); err != nil {
		return err
	}
	e.Updated = time.Now().UnixMilli()
	return nil
}

func (s *NotesService) read(id string) (string, []string, string, error) {
	e := s.idx[id]
	if e == nil {
		return "", nil, "", errNotFound
	}
	raw, err := os.ReadFile(s.file(e.Path))
	if err != nil {
		return "", nil, "", err
	}
	_, tags, _, body := parse(string(raw))
	return e.Path, tags, body, nil
}

// ---- bound to the frontend ----

func (s *NotesService) Tree() (Tree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Tree{Notes: []NoteMeta{}, Folders: []string{}}
	for id, e := range s.idx {
		t.Notes = append(t.Notes, NoteMeta{id, e.Path, e.Tags, e.Updated})
	}
	sort.Slice(t.Notes, func(i, j int) bool { return strings.ToLower(t.Notes[i].Path) < strings.ToLower(t.Notes[j].Path) })
	filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != s.root {
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			rel, _ := filepath.Rel(s.root, p)
			t.Folders = append(t.Folders, filepath.ToSlash(rel))
		}
		return nil
	})
	return t, nil
}

func (s *NotesService) Get(id string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, tags, body, err := s.read(id)
	return Note{id, p, tags, body}, err
}

func (s *NotesService) Create(folder, title string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, err := cleanPath(path.Join(folder, title))
	if err != nil {
		return Note{}, err
	}
	id, e := newID(), &entry{Path: s.unique(rel, ""), Tags: []string{}}
	if err := s.write(id, e, ""); err != nil {
		return Note{}, err
	}
	s.idx[id] = e
	s.touch(id)
	return Note{id, e.Path, e.Tags, ""}, s.saveState()
}

// Save persists content+tags and returns the new updated time (unix ms).
func (s *NotesService) Save(id, content string, tags []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.idx[id]
	if e == nil {
		return 0, errNotFound
	}
	tags = cleanTags(tags)
	if _, oldTags, oldBody, err := s.read(id); err == nil && oldBody == content && strings.Join(oldTags, ",") == strings.Join(tags, ",") {
		return e.Updated, nil // nothing changed: don't create a sync revision
	}
	e.Tags = tags
	if err := s.write(id, e, content); err != nil {
		return 0, err
	}
	s.touch(id)
	return e.Updated, s.saveState()
}

func (s *NotesService) Move(id, newPath string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.idx[id]
	if e == nil {
		return "", errNotFound
	}
	rel, err := cleanPath(newPath)
	if err != nil {
		return "", err
	}
	if rel = s.unique(rel, id); rel == e.Path {
		return rel, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.file(rel)), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(s.file(e.Path), s.file(rel)); err != nil {
		return "", err
	}
	e.Path = rel
	s.touch(id)
	return rel, s.saveState()
}

func (s *NotesService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.idx[id]
	if e == nil {
		return errNotFound
	}
	if err := os.Remove(s.file(e.Path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	delete(s.idx, id)
	if n := s.st.Notes[id]; n == nil || n.BaseRev == 0 {
		delete(s.st.Notes, id) // never reached the server
	} else {
		n.Deleted, n.Dirty = true, true
	}
	return s.saveState()
}

func (s *NotesService) CreateFolder(p string) error {
	rel, err := cleanPath(p)
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(s.root, filepath.FromSlash(rel)), 0o755)
}

func (s *NotesService) DeleteFolder(p string) error {
	rel, err := cleanPath(p)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.root, filepath.FromSlash(rel))); err != nil {
		return errors.New("folder is not empty")
	}
	return nil
}

func (s *NotesService) RenameFolder(oldP, newP string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := cleanPath(oldP)
	if err != nil {
		return err
	}
	n, err := cleanPath(newP)
	if err != nil {
		return err
	}
	dst := filepath.Join(s.root, filepath.FromSlash(n))
	if _, err := os.Stat(dst); err == nil {
		return errors.New("a folder with that name already exists")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(s.root, filepath.FromSlash(o)), dst); err != nil {
		return err
	}
	for id, e := range s.idx {
		if strings.HasPrefix(e.Path, o+"/") {
			e.Path = n + e.Path[len(o):]
			s.touch(id)
		}
	}
	return s.saveState()
}

func (s *NotesService) Tags() []TagCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, out := map[string]int{}, []TagCount{}
	for _, e := range s.idx {
		for _, t := range e.Tags {
			c[t]++
		}
	}
	for t, n := range c {
		out = append(out, TagCount{t, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}

// ---- sync settings / trigger ----

func (s *NotesService) GetServer() ServerSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ServerSettings{s.st.ServerURL, s.st.Token != ""}
}

// SetServer saves the sync server; an empty token keeps the existing one,
// an empty url disables sync.
func (s *NotesService) SetServer(url, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.ServerURL = strings.TrimRight(strings.TrimSpace(url), "/")
	if token != "" {
		s.st.Token = token
	}
	if s.st.ServerURL == "" {
		s.st.Token = ""
	}
	return s.saveState()
}

func (s *NotesService) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SyncStatus{Configured: s.st.ServerURL != "", LastSync: s.last, LastError: s.lastErr}
	for _, n := range s.st.Notes {
		if n.Dirty {
			st.Pending++
		}
	}
	return st
}

// SyncNow runs one push+pull cycle. Errors are reported in the status.
func (s *NotesService) SyncNow() SyncStatus {
	if !s.syncing.TryLock() {
		return s.Status()
	}
	defer s.syncing.Unlock()
	s.mu.Lock()
	url, tok := s.st.ServerURL, s.st.Token
	s.mu.Unlock()
	if url == "" {
		return s.Status()
	}
	c := &syncer.Client{BaseURL: url, Token: tok, HTTP: &http.Client{Timeout: 30 * time.Second}, Local: local{s}}
	err := c.Sync(context.Background())
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr, s.last = "", time.Now().Unix()
	}
	s.mu.Unlock()
	return s.Status()
}

// ---- syncer.Local (unexported adapter so it isn't bound to the frontend) ----

type local struct{ s *NotesService }

func (l local) Dirty() ([]syncer.Pending, error) {
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []syncer.Pending
	for id, n := range s.st.Notes {
		if !n.Dirty {
			continue
		}
		if n.Deleted {
			out = append(out, syncer.Pending{Note: proto.Note{ID: id, Deleted: true, Tags: []string{}}, BaseRev: n.BaseRev})
			continue
		}
		p, tags, body, err := s.read(id)
		if err != nil {
			continue
		}
		out = append(out, syncer.Pending{Note: proto.Note{ID: id, Path: p, Tags: tags, Content: body}, BaseRev: n.BaseRev})
	}
	return out, nil
}

func (l local) IsDirty(id string) bool {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	n := l.s.st.Notes[id]
	return n != nil && n.Dirty
}

func (l local) Apply(r proto.Note) error {
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.idx[r.ID]
	if r.Deleted {
		if e != nil {
			os.Remove(s.file(e.Path))
			delete(s.idx, r.ID)
		}
		s.st.Notes[r.ID] = &ns{BaseRev: r.Rev, Deleted: true}
		return s.saveState()
	}
	rel, err := cleanPath(r.Path)
	if err != nil {
		rel = "Untitled " + r.ID[:8]
	}
	rel = s.unique(rel, r.ID)
	if e == nil {
		e = &entry{}
	} else if e.Path != rel {
		if err := os.MkdirAll(filepath.Dir(s.file(rel)), 0o755); err != nil {
			return err
		}
		if err := os.Rename(s.file(e.Path), s.file(rel)); err != nil {
			return err
		}
	}
	e.Path, e.Tags = rel, cleanTags(r.Tags)
	if err := s.write(r.ID, e, r.Content); err != nil {
		return err
	}
	s.idx[r.ID] = e
	s.st.Notes[r.ID] = &ns{BaseRev: r.Rev}
	return s.saveState()
}

func (l local) MarkPushed(r proto.Note) error {
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.st.Notes[r.ID]
	if n == nil {
		n = &ns{}
		s.st.Notes[r.ID] = n
	}
	n.BaseRev = r.Rev
	if r.Deleted {
		n.Dirty = !n.Deleted
	} else if _, tags, body, err := s.read(r.ID); err == nil && proto.Hash(body) == r.Hash && strings.Join(tags, ",") == strings.Join(cleanTags(r.Tags), ",") {
		n.Dirty = false // edited again while pushing? then it stays dirty
	}
	return s.saveState()
}

func (l local) SaveConflictCopy(c proto.Note) error {
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	rel, err := cleanPath(c.Path)
	if err != nil {
		rel = "Conflict " + id[:8]
	}
	e := &entry{Path: s.unique(rel, ""), Tags: cleanTags(c.Tags)}
	if err := s.write(id, e, c.Content); err != nil {
		return err
	}
	s.idx[id] = e
	s.touch(id)
	return s.saveState()
}

func (l local) Cursor() int64 {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	return l.s.st.Cursor
}

func (l local) SetCursor(c int64) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	l.s.st.Cursor = c
	return l.s.saveState()
}
