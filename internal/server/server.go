// Package server is the central sync service: an append-only, checksummed
// journal (never rewritten in place) plus optimistic-concurrency pushes.
package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"sync"

	"go.dalw.in/holocron/internal/proto"
)

var (
	ErrConflict = errors.New("conflict")
	ErrBadHash  = errors.New("content hash mismatch")
)

type Store struct {
	mu    sync.Mutex
	f     *os.File
	size  int64
	seq   int64
	notes map[string]proto.Note
}

// OpenStore replays the journal. A torn last line (crash mid-write) or any
// line failing its CRC ends the replay and is truncated away.
func OpenStore(path string) (*Store, error) {
	s := &Store{notes: map[string]proto.Note{}}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	good := 0
	for good < len(data) {
		i := bytes.IndexByte(data[good:], '\n')
		if i < 0 {
			break
		}
		n, ok := parseLine(data[good : good+i])
		if !ok {
			break
		}
		s.notes[n.ID] = n
		s.seq = max(s.seq, n.Seq)
		good += i + 1
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(int64(good)); err != nil {
		return nil, err
	}
	s.f, s.size = f, int64(good)
	return s, nil
}

func (s *Store) Close() error { return s.f.Close() }

func encodeLine(n proto.Note) []byte {
	js, _ := json.Marshal(n)
	return append([]byte(fmt.Sprintf("%08x ", crc32.ChecksumIEEE(js))), append(js, '\n')...)
}

func parseLine(b []byte) (n proto.Note, ok bool) {
	if len(b) < 10 || b[8] != ' ' {
		return n, false
	}
	want, err := strconv.ParseUint(string(b[:8]), 16, 32)
	if err != nil || crc32.ChecksumIEEE(b[9:]) != uint32(want) {
		return n, false
	}
	return n, json.Unmarshal(b[9:], &n) == nil
}

func same(a, b proto.Note) bool {
	return a.Hash == b.Hash && a.Path == b.Path && a.Deleted == b.Deleted && slices.Equal(a.Tags, b.Tags)
}

// Push applies an edit only if it was based on the note's current revision.
// On conflict it returns the server's current note and changes nothing.
func (s *Store) Push(req proto.PushRequest) (proto.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := req.Note
	if n.ID == "" {
		return n, errors.New("missing id")
	}
	if !n.Deleted && proto.Hash(n.Content) != n.Hash {
		return n, ErrBadHash
	}
	cur, exists := s.notes[n.ID]
	if cur.Rev != req.BaseRev { // exists && stale, or !exists && base != 0
		if exists && same(cur, n) {
			return cur, nil // retry of a push whose response was lost
		}
		return cur, ErrConflict
	}
	n.Rev, n.Seq = cur.Rev+1, s.seq+1
	line := encodeLine(n)
	if _, err := s.f.Write(line); err != nil {
		s.f.Truncate(s.size)
		return n, err
	}
	if err := s.f.Sync(); err != nil { // durable before we acknowledge
		return n, err
	}
	s.size += int64(len(line))
	s.seq, s.notes[n.ID] = n.Seq, n
	return n, nil
}

func (s *Store) Changes(since int64) proto.Changes {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := proto.Changes{Notes: []proto.Note{}, Cursor: since}
	for _, n := range s.notes {
		if n.Seq > since {
			out.Notes = append(out.Notes, n)
		}
	}
	sort.Slice(out.Notes, func(i, j int) bool { return out.Notes[i].Seq < out.Notes[j].Seq })
	if l := len(out.Notes); l > 0 {
		out.Cursor = out.Notes[l-1].Seq
	}
	return out
}

func Handler(s *Store, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/changes", func(w http.ResponseWriter, r *http.Request) {
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		writeJSON(w, 200, s.Changes(since))
	})
	mux.HandleFunc("PUT /v1/notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req proto.PushRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&req); err != nil || req.Note.ID != r.PathValue("id") {
			http.Error(w, "bad request", 400)
			return
		}
		n, err := s.Push(req)
		switch {
		case errors.Is(err, ErrConflict):
			writeJSON(w, 409, n)
		case err != nil:
			http.Error(w, err.Error(), 400)
		default:
			writeJSON(w, 200, n)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
