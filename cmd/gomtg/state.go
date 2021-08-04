package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frizinak/gomtg/mtgjson"
)

type State struct {
	Mode       Mode
	PrevMode   Mode
	FilterSet  mtgjson.SetID
	Options    []mtgjson.Card
	Local      []LocalCard
	PageOffset int

	Selection []mtgjson.Card
	Tagging   []Tagging
}

func (s State) Equal(o State) bool {
	if s.Mode != o.Mode ||
		s.PrevMode != o.PrevMode ||
		s.FilterSet != o.FilterSet ||
		len(s.Selection) != len(o.Selection) ||
		len(s.Tagging) != len(o.Tagging) ||
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

	for i := range s.Tagging {
		if !s.Tagging[i].Equal(o.Tagging[i]) {
			return false
		}
	}

	return true
}

func (s State) String(db *DB, colors Colors) string {
	data := []string{s.StringShort()}
	for _, c := range s.Selection {
		data = append(
			data,
			fmt.Sprintf(
				"  \u2514 %s",
				cardsString(db, []mtgjson.Card{c}, colors, false)[0],
			),
		)
	}

	tagsAdd := make(map[string]int)
	tagsRem := make(map[string]int)
	for _, c := range s.Tagging {
		for add := range c.tagsAdd {
			tagsAdd[add]++
		}
		for rem := range c.tagsRem {
			tagsRem[rem]++
		}
	}

	for tag, amount := range tagsAdd {
		data = append(
			data,
			fmt.Sprintf(" \u2514 added tag '%s' to %d cards", tag, amount),
		)
	}
	for tag, amount := range tagsRem {
		data = append(
			data,
			fmt.Sprintf(" \u2514 removed tag '%s' from %d cards", tag, amount),
		)
	}

	return strings.Join(data, "\n")
}

func (s State) StringShort() string {
	d := make([]string, 3, 4)
	d[0] = fmt.Sprintf("mode:%s", s.Mode)
	d[1] = fmt.Sprintf("set:%s", s.FilterSet)
	d[2] = fmt.Sprintf("selected:%d", len(s.Selection))
	d = append(d, string(s.Mode))
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
	case ModeSelect:
		return true
	}

	return false
}

func (m Mode) ValidInput() bool {
	_, ok := AllInputModes[m]
	return ok
}

const (
	ModeAdd        Mode = "add"
	ModeSelect     Mode = "select"
	ModeCollection Mode = "collection"
	ModeSearch     Mode = "search"
)

var AllInputModes = map[Mode]struct{}{
	ModeAdd:        {},
	ModeCollection: {},
	ModeSearch:     {},
}

type newTags map[string]struct{}

func (t newTags) Slice() []string {
	n := make([]string, 0, len(t))
	for i := range t {
		n = append(n, i)
	}
	sort.Strings(n)
	return n
}

type Tagging struct {
	*Card
	tagsAdd newTags
	tagsRem newTags
}

func NewTagging(c *Card) Tagging {
	return Tagging{
		c,
		make(newTags),
		make(newTags),
	}
}

func (t Tagging) Add(add bool, tag string) {
	if add {
		t.tagsAdd[tag] = struct{}{}
		delete(t.tagsRem, tag)
		return
	}
	t.tagsRem[tag] = struct{}{}
	delete(t.tagsAdd, tag)
}

func (t Tagging) Commit() {
	t.Card.Untag(t.tagsRem.Slice())
	t.Card.Tag(t.tagsAdd.Slice())
}

func (t Tagging) NewTags() (added []string, removed []string) {
	added, removed = t.tagsAdd.Slice(), t.tagsRem.Slice()
	return
}

func (t Tagging) Equal(o Tagging) bool {
	if t.uuid != o.uuid {
		return false
	}
	if t.NewTagsString() != o.NewTagsString() {
		return false
	}

	return true
}

func (t Tagging) NewTagsString() string {
	add, rem := t.NewTags()
	if len(add) == 0 && len(rem) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"add: %s, remove: %s",
		strings.Join(add, ","),
		strings.Join(rem, ","),
	)

}

type LocalCard struct {
	*Card
	Index int
}

func NewLocalCard(c *Card, ix int) LocalCard {
	return LocalCard{
		Card:  c,
		Index: ix,
	}
}
