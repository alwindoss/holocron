// Package proto holds the wire types shared by the app and the sync server.
package proto

import (
	"crypto/sha256"
	"encoding/hex"
)

// Note is one markdown note. Rev and Seq are assigned only by the server.
type Note struct {
	ID      string   `json:"id"`   // client-generated UUID; stable across renames/moves
	Path    string   `json:"path"` // "Work/Ideas/Launch plan" (folders + title)
	Tags    []string `json:"tags"`
	Content string   `json:"content"` // markdown
	Hash    string   `json:"hash"`    // sha256 of Content, verified by the server
	Rev     int64    `json:"rev"`     // per-note revision (optimistic concurrency)
	Seq     int64    `json:"seq"`     // global change sequence (pull cursor)
	Deleted bool     `json:"deleted"` // tombstone
}

type PushRequest struct {
	BaseRev int64 `json:"base_rev"` // server rev this edit was based on (0 = new note)
	Note    Note  `json:"note"`
}

type Changes struct {
	Notes  []Note `json:"notes"`
	Cursor int64  `json:"cursor"`
}

func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
