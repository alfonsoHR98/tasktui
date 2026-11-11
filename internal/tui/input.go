package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// input es una línea de texto editable con cursor (se dibuja con el carácter
// bajo el cursor en vídeo inverso, así siempre se ve dónde se escribe).
type input struct {
	buf []rune
	cur int
}

func (in *input) set(s string) {
	in.buf = []rune(s)
	in.cur = len(in.buf)
}

func (in *input) text() string { return string(in.buf) }

func (in *input) handle(k tea.KeyMsg) {
	switch k.Type {
	case tea.KeyCtrlA, tea.KeyHome:
		in.cur = 0
	case tea.KeyCtrlE, tea.KeyEnd:
		in.cur = len(in.buf)
	case tea.KeyCtrlU:
		in.buf = in.buf[in.cur:]
		in.cur = 0
	case tea.KeyCtrlW:
		for in.cur > 0 && in.buf[in.cur-1] == ' ' {
			in.cur--
			in.buf = append(in.buf[:in.cur], in.buf[in.cur+1:]...)
		}
		for in.cur > 0 && in.buf[in.cur-1] != ' ' {
			in.cur--
			in.buf = append(in.buf[:in.cur], in.buf[in.cur+1:]...)
		}
	case tea.KeyBackspace:
		if in.cur > 0 {
			in.cur--
			in.buf = append(in.buf[:in.cur], in.buf[in.cur+1:]...)
		}
	case tea.KeyDelete:
		if in.cur < len(in.buf) {
			in.buf = append(in.buf[:in.cur], in.buf[in.cur+1:]...)
		}
	case tea.KeyLeft:
		if in.cur > 0 {
			in.cur--
		}
	case tea.KeyRight:
		if in.cur < len(in.buf) {
			in.cur++
		}
	case tea.KeySpace:
		in.insert([]rune{' '})
	case tea.KeyRunes:
		in.insert(k.Runes)
	}
}

func (in *input) insert(rs []rune) {
	out := make([]rune, 0, len(in.buf)+len(rs))
	out = append(out, in.buf[:in.cur]...)
	out = append(out, rs...)
	out = append(out, in.buf[in.cur:]...)
	in.buf = out
	in.cur += len(rs)
}
