package notes

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path"
	"strings"
)

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// parse splits "---\nid: ..\ntags: [a, b]\n---\nbody". Unknown front-matter
// lines are returned in extra so user metadata is preserved on rewrite.
func parse(raw string) (id string, tags []string, extra, body string) {
	tags = []string{}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return "", tags, "", raw
	}
	end := strings.Index(raw[3:], "\n---\n")
	if end < 0 {
		return "", tags, "", raw
	}
	head, body := raw[4:3+end+1], raw[3+end+5:]
	for _, l := range strings.Split(strings.TrimSuffix(head, "\n"), "\n") {
		k, v, ok := strings.Cut(l, ":")
		switch k = strings.TrimSpace(k); {
		case ok && k == "id":
			id = strings.TrimSpace(v)
		case ok && k == "tags":
			tags = cleanTags(strings.Split(strings.Trim(strings.TrimSpace(v), "[]"), ","))
		case strings.TrimSpace(l) != "":
			extra += l + "\n"
		}
	}
	return
}

func render(id string, tags []string, extra, body string) string {
	return "---\nid: " + id + "\ntags: [" + strings.Join(tags, ", ") + "]\n" + extra + "---\n" + body
}

func cleanTags(in []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(strings.Trim(strings.TrimSpace(t), `#"'`))
		t = strings.NewReplacer(",", "", "[", "", "]", "", "\n", " ").Replace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// cleanPath validates "Folder/Sub/Title" (no traversal, no hidden parts).
func cleanPath(p string) (string, error) {
	var parts []string
	for _, s := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if s == "." || s == ".." || strings.HasPrefix(s, ".") || strings.ContainsAny(s, "<>:\"|?*\x00") {
			return "", errors.New("invalid name: " + s)
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "", errors.New("name is empty")
	}
	return path.Join(parts...), nil
}
