package main

import (
	"fmt"
	"strings"

	"github.com/frizinak/gomtg/mtgjson"
)

type State struct {
	Mode      Mode
	PrevMode  Mode
	FilterSet mtgjson.SetID
	Query     string
	Selection []mtgjson.Card
	Options   []mtgjson.Card
	Local     []LocalCard
}

func (s State) Equal(o State) bool {
	if s.Mode != o.Mode ||
		s.PrevMode != o.PrevMode ||
		s.FilterSet != o.FilterSet ||
		s.Query != o.Query ||
		len(s.Selection) != len(o.Selection) ||
		len(s.Options) != len(o.Options) {
		return false
	}

	for i := range s.Selection {
		if s.Selection[i].UUID != o.Selection[i].UUID {
			return false
		}
	}
	for i := range s.Options {
		if s.Options[i].UUID != o.Options[i].UUID {
			return false
		}
	}

	return true
}

func (s State) String() string {
	data := []string{s.StringShort()}
	for _, c := range s.Selection {
		data = append(data, fmt.Sprintf("  \u2514 %s", cardString(c)))
	}

	return strings.Join(data, "\n")
}

func (s State) StringShort() string {
	d := make([]string, 3, 4)
	d[0] = fmt.Sprintf("mode:%s", s.Mode)
	d[1] = fmt.Sprintf("set:%s", s.FilterSet)
	d[2] = fmt.Sprintf("selected:%d", len(s.Selection))
	// if s.Query != "" {
	// 	d = append(d, fmt.Sprintf("query:'%s'", s.Query))
	// }
	if len(s.Options) != 0 {
		d = append(d, fmt.Sprintf("options:%d", len(s.Options)))
	}
	return strings.Join(d, " ")
}

type Mode string

func (m Mode) Valid() bool {
	if m.ValidInput() {
		return true
	}

	switch m {
	case ModeSel:
		return true
	}

	return false
}

func (m Mode) ValidInput() bool {
	switch m {
	case ModeAdd:
		return true
	case ModeCol:
		return true
	case ModeSearch:
		return true
	}
	return false
}

const (
	ModeAdd    Mode = "add"
	ModeSel         = "sel"
	ModeCol         = "col"
	ModeSearch      = "search"
)

var AllInputModes = []Mode{
	ModeAdd,
	ModeCol,
	ModeSearch,
}

type LocalCard struct {
	*Card
	Index int
}
