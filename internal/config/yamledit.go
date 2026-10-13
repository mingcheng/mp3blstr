package config

import (
	"errors"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// errUnsupported reports a document layout that editText cannot change
// line by line (flow style, multi-line values, tags, ...).
var errUnsupported = errors.New("unsupported layout")

// editText applies settings to the YAML text of a configuration file by
// editing single lines, so that the formatting, comments and blank lines of
// the file are kept. Existing values are replaced in place, removed
// settings lose their line and new settings are added at the end of their
// section (or of the file, for a new section).
func editText(text string, settings []Setting) (string, error) {
	e := &textEditor{}
	if t := strings.TrimSuffix(text, "\n"); t != "" {
		e.lines = strings.Split(t, "\n")
	}
	for _, s := range settings {
		segs := strings.Split(strings.ToLower(s.Name), ".")
		var err error
		if s.Value == "" {
			err = e.remove(segs)
		} else {
			err = e.set(segs, &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlTag(s.Name), Value: s.Value})
		}
		if err != nil {
			return "", err
		}
	}
	out := strings.Join(e.lines, "\n") + "\n"
	if !settingsApplied(out, settings) {
		return "", errUnsupported
	}
	return out, nil
}

type textEditor struct{ lines []string }

// root parses the current text and returns its top level mapping, or nil
// for an empty document.
func (e *textEditor) root() (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(e.lines, "\n")), &doc); err != nil {
		return nil, err
	}
	root := documentRoot(&doc)
	if root == nil || isNull(root) {
		return nil, nil
	}
	if root.Kind != yaml.MappingNode || !isBlock(root) {
		return nil, errUnsupported
	}
	return root, nil
}

func (e *textEditor) set(segs []string, val *yaml.Node) error {
	enc, ok := encodeScalar(val)
	if !ok {
		return errUnsupported
	}
	m, err := e.root()
	if err != nil {
		return err
	}
	if m == nil {
		e.appendSection(0, segs, enc)
		return nil
	}
	for i, seg := range segs {
		j := lookup(m, seg)
		if j < 0 {
			if i == 0 {
				e.appendSection(m.Content[0].Column-1, segs, enc)
				return nil
			}
			last, ok := lastLine(m)
			if !ok {
				return errUnsupported
			}
			e.insert(last, m.Content[0].Column-1, segs[i:], enc)
			return nil
		}
		k, v := m.Content[j-1], m.Content[j]
		if i == len(segs)-1 {
			return e.replace(v, enc)
		}
		switch {
		case isNull(v) && v.Value == "":
			// An empty section ("state:"): add the setting below it.
			e.insert(k.Line, k.Column+1, segs[i+1:], enc)
			return nil
		case v.Kind != yaml.MappingNode || !isBlock(v):
			return errUnsupported
		}
		m = v
	}
	return nil
}

func (e *textEditor) remove(segs []string) error {
	m, err := e.root()
	if err != nil || m == nil {
		return err
	}
	for i, seg := range segs {
		j := lookup(m, seg)
		if j < 0 {
			return nil
		}
		k, v := m.Content[j-1], m.Content[j]
		if i < len(segs)-1 {
			if isNull(v) {
				return nil
			}
			if v.Kind != yaml.MappingNode || !isBlock(v) {
				return errUnsupported
			}
			m = v
			continue
		}
		if v.Kind != yaml.ScalarNode || v.Line != k.Line || !e.onlyIndented(k) {
			return errUnsupported
		}
		if _, _, ok := e.scalarSpan(v); !ok {
			return errUnsupported
		}
		e.lines = append(e.lines[:k.Line-1], e.lines[k.Line:]...)
	}
	return nil
}

// replace overwrites the text of the scalar v with enc, keeping the rest of
// its line (such as a comment).
func (e *textEditor) replace(v *yaml.Node, enc string) error {
	if v.Kind != yaml.ScalarNode || (isNull(v) && v.Value == "") {
		return errUnsupported
	}
	start, end, ok := e.scalarSpan(v)
	if !ok {
		return errUnsupported
	}
	l := e.lines[v.Line-1]
	e.lines[v.Line-1] = l[:start] + enc + l[end:]
	return nil
}

// insert adds the setting at the path segs (creating the missing sections)
// after line, indented by indent columns.
func (e *textEditor) insert(line, indent int, segs []string, enc string) {
	block := settingLines(indent, segs, enc)
	e.lines = append(e.lines[:line], append(block, e.lines[line:]...)...)
}

// appendSection adds a new top level section at the end of the file,
// separated from the previous content by a blank line.
func (e *textEditor) appendSection(indent int, segs []string, enc string) {
	if len(e.lines) == 0 {
		e.lines = append(e.lines, "# "+savedComment)
	}
	if strings.TrimSpace(e.lines[len(e.lines)-1]) != "" {
		e.lines = append(e.lines, "")
	}
	e.lines = append(e.lines, settingLines(indent, segs, enc)...)
}

// settingLines renders "a:\n  b: enc" for segs [a b].
func settingLines(indent int, segs []string, enc string) []string {
	var out []string
	for i, seg := range segs {
		l := strings.Repeat(" ", indent+2*i) + seg + ":"
		if i == len(segs)-1 {
			l += " " + enc
		}
		out = append(out, l)
	}
	return out
}

// onlyIndented reports whether nothing but white space precedes n on its
// line.
func (e *textEditor) onlyIndented(n *yaml.Node) bool {
	off, ok := byteOffset(e.lines[n.Line-1], n.Column)
	return ok && strings.TrimSpace(e.lines[n.Line-1][:off]) == ""
}

// scalarSpan returns the byte range of the single-line scalar v on its
// line; ok is false for scalars that span lines or carry a tag or anchor.
func (e *textEditor) scalarSpan(v *yaml.Node) (start, end int, ok bool) {
	if v.Line < 1 || v.Line > len(e.lines) || v.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
		return 0, 0, false
	}
	l := e.lines[v.Line-1]
	start, ok = byteOffset(l, v.Column)
	if !ok || start >= len(l) {
		return 0, 0, false
	}
	switch l[start] {
	case '!', '&', '*', '|', '>':
		return 0, 0, false
	case '\'':
		for i := start + 1; i < len(l); i++ {
			if l[i] == '\'' {
				if i+1 < len(l) && l[i+1] == '\'' {
					i++
					continue
				}
				return start, i + 1, true
			}
		}
		return 0, 0, false
	case '"':
		for i := start + 1; i < len(l); i++ {
			switch l[i] {
			case '\\':
				i++
			case '"':
				return start, i + 1, true
			}
		}
		return 0, 0, false
	}
	end = len(l)
	if i := strings.Index(l[start:], " #"); i >= 0 {
		end = start + i
	}
	if i := strings.Index(l[start:end], "\t#"); i >= 0 {
		end = start + i
	}
	end = start + len(strings.TrimRight(l[start:end], " \t"))
	if l[start:end] != v.Value {
		return 0, 0, false // continued on the next lines
	}
	return start, end, true
}

// lastLine returns the last line taken by the block collection n.
func lastLine(n *yaml.Node) (int, bool) {
	switch {
	case n.Kind == yaml.AliasNode:
		return n.Line, true
	case n.Kind == yaml.ScalarNode:
		if n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || strings.Contains(n.Value, "\n") {
			return 0, false
		}
		return n.Line, true
	case !isBlock(n):
		return 0, false
	case len(n.Content) == 0:
		return n.Line, true
	}
	last := n.Content[len(n.Content)-1]
	l, ok := lastLine(last)
	if n.Kind == yaml.MappingNode {
		l = max(l, n.Content[len(n.Content)-2].Line)
	}
	return l, ok
}

func isBlock(n *yaml.Node) bool { return n.Style&yaml.FlowStyle == 0 }

// byteOffset converts the 1-based character column of a YAML mark into a
// byte offset in line.
func byteOffset(line string, column int) (int, bool) {
	off := 0
	for c := 1; c < column; c++ {
		if off >= len(line) {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(line[off:])
		off += size
	}
	return off, true
}

// encodeScalar renders a scalar for a single line.
func encodeScalar(n *yaml.Node) (string, bool) {
	b, err := yaml.Marshal(n)
	s := strings.TrimSuffix(string(b), "\n")
	return s, err == nil && !strings.Contains(s, "\n")
}

// settingsApplied verifies that text holds the settings: a set value reads
// back unchanged, a removed one is absent.
func settingsApplied(text string, settings []Setting) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return false
	}
	root := documentRoot(&doc)
	for _, s := range settings {
		n := root
		for _, seg := range strings.Split(strings.ToLower(s.Name), ".") {
			if n == nil || n.Kind != yaml.MappingNode {
				n = nil
				break
			}
			i := lookup(n, seg)
			if i < 0 {
				n = nil
				break
			}
			n = resolve(n.Content[i])
		}
		if s.Value == "" {
			if n != nil && !isNull(n) {
				return false
			}
		} else if n == nil || n.Kind != yaml.ScalarNode || n.Value != s.Value {
			return false
		}
	}
	return true
}
