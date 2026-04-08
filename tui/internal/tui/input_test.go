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

func TestCalcHeight_Empty(t *testing.T) {
	m := newTestInput(80)
	if h := m.calcHeight(); h != 1 {
		t.Errorf("empty input: expected height 1, got %d", h)
	}
}

func TestCalcHeight_ShortText(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("hello world")
	if h := m.calcHeight(); h != 1 {
		t.Errorf("short text: expected height 1, got %d", h)
	}
}

func TestCalcHeight_WrappingText(t *testing.T) {
	m := newTestInput(40) // textarea width = 40 - 10 = 30
	// 60 chars of words — should wrap to 2+ lines on a 30-col textarea
	m.textarea.SetValue("the quick brown fox jumps over the lazy dog again and again")
	h := m.calcHeight()
	if h < 2 {
		t.Errorf("wrapping text on 30-col textarea: expected height >= 2, got %d", h)
	}
}

func TestCalcHeight_ExplicitNewlines(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("line one\nline two\nline three")
	if h := m.calcHeight(); h != 3 {
		t.Errorf("3 explicit lines: expected height 3, got %d", h)
	}
}

func TestCalcHeight_MaxSix(t *testing.T) {
	m := newTestInput(80)
	m.textarea.SetValue("a\nb\nc\nd\ne\nf\ng\nh")
	if h := m.calcHeight(); h != 6 {
		t.Errorf("8 lines: expected capped height 6, got %d", h)
	}
}

func TestCalcHeight_CJK(t *testing.T) {
	m := newTestInput(40) // textarea width = 30
	// 20 CJK chars × 2 visual cols each = 40 visual cols
	// 40 / 30 = ceil(1.33) = 2 visual lines
	m.textarea.SetValue(strings.Repeat("\u4f60", 20))
	h := m.calcHeight()
	if h != 2 {
		t.Errorf("CJK wrapping on 30-col textarea: expected height 2, got %d", h)
	}
}

func TestVisualLineCount_Basic(t *testing.T) {
	// Empty string → 1 line
	if h := visualLineCount("", 30); h != 1 {
		t.Errorf("empty: expected 1, got %d", h)
	}
	// Short line fits in width → 1 line
	if h := visualLineCount("hello", 30); h != 1 {
		t.Errorf("short: expected 1, got %d", h)
	}
	// Exact fit → 1 line
	if h := visualLineCount("abcdefghij", 10); h != 1 {
		t.Errorf("exact fit: expected 1, got %d", h)
	}
	// Overflow by 1 → 2 lines
	if h := visualLineCount("abcdefghijk", 10); h != 2 {
		t.Errorf("overflow 1: expected 2, got %d", h)
	}
	// 20 CJK chars × 2 cols = 40 cols, / 30 → ceil(1.33) = 2
	if h := visualLineCount(strings.Repeat("\u4f60", 20), 30); h != 2 {
		t.Errorf("20 CJK on 30-col: expected 2, got %d", h)
	}
	// 20 CJK chars × 2 cols = 40 cols, / 20 → ceil(2.0) = 2
	if h := visualLineCount(strings.Repeat("\u4f60", 20), 20); h != 2 {
		t.Errorf("20 CJK on 20-col: expected 2, got %d", h)
	}
	// 35 CJK chars × 2 cols = 70 cols, / 30 → ceil(2.34) = 3
	if h := visualLineCount(strings.Repeat("\u4f60", 35), 30); h != 3 {
		t.Errorf("35 CJK on 30-col: expected 3, got %d", h)
	}
}

func TestVisualLineCount_Multiline(t *testing.T) {
	// Two logical lines: first wraps to 2, second fits → total 3
	m := newTestInput(40)
	m.textarea.SetValue(strings.Repeat("\u4f60", 20) + "\nhi")
	h := m.calcHeight()
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
