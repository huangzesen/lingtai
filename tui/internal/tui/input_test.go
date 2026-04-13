package tui

import (
	"strings"
	"testing"
)

func newTestInput(width int) *InputModel {
	m := NewInputModel("")
	m.SetWidth(width)
	return &m
}

func TestLineCount_Empty(t *testing.T) {
	m := newTestInput(80)
	if h := m.LineCount(); h != 1 {
		t.Errorf("empty input: expected height 1, got %d", h)
	}
}

func TestLineCount_ShortText(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("hello world")
	if h := m.LineCount(); h != 1 {
		t.Errorf("short text: expected height 1, got %d", h)
	}
}

func TestLineCount_WrappingText(t *testing.T) {
	m := newTestInput(40) // textarea width = 40 - 10 = 30
	// 60 chars of words — should soft-wrap to 2+ visual lines on a 30-col textarea
	m.textarea.SetValue("the quick brown fox jumps over the lazy dog again and again")
	h := m.LineCount()
	if h < 2 {
		t.Errorf("wrapping text on 30-col textarea: expected height >= 2, got %d", h)
	}
}

func TestLineCount_ExplicitNewlines(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("line one\nline two\nline three")
	if h := m.LineCount(); h != 3 {
		t.Errorf("3 explicit lines: expected height 3, got %d", h)
	}
}

func TestLineCount_MaxSix(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("a\nb\nc\nd\ne\nf\ng\nh")
	if h := m.LineCount(); h != 6 {
		t.Errorf("8 lines: expected capped height 6, got %d", h)
	}
}

func TestLineCount_CJK(t *testing.T) {
	m := newTestInput(40) // textarea width = 30
	// 20 CJK chars × 2 visual cols each = 40 visual cols
	// 40 / 30 = ceil(1.33) = 2 visual lines
	m.textarea.SetValue(strings.Repeat("\u4f60", 20))
	h := m.LineCount()
	if h != 2 {
		t.Errorf("CJK wrapping on 30-col textarea: expected height 2, got %d", h)
	}
}

func TestLineCount_Multiline(t *testing.T) {
	// Two logical lines: first wraps to 2 visual lines, second fits → total 3
	m := newTestInput(40)
	m.textarea.SetValue(strings.Repeat("\u4f60", 20) + "\nhi")
	h := m.LineCount()
	if h != 3 {
		t.Errorf("multiline CJK: expected 3, got %d", h)
	}
}

func TestView_HasBottomBorder(t *testing.T) {
	m := NewInputModel("")
	m.SetWidth(40)
	view := m.View()
	lines := strings.Split(view, "\n")
	lastLine := lines[len(lines)-1]
	// Bottom border should be a line of "─" characters
	trimmed := strings.TrimRight(lastLine, "─")
	if trimmed != "" || len(lastLine) == 0 {
		t.Errorf("expected bottom border of ─ chars, got last line: %q", lastLine)
	}
}

func TestNoHardNewlinesFromWrapping(t *testing.T) {
	m := newTestInput(40) // textarea width = 30
	// Long text that would have triggered the old autoWrap
	text := "the quick brown fox jumps over the lazy dog again and again and again"
	m.textarea.SetValue(text)
	// The value should remain unchanged — no inserted newlines
	if got := m.textarea.Value(); got != text {
		t.Errorf("textarea value should not have hard newlines from wrapping\ngot:  %q\nwant: %q", got, text)
	}
}
