package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frizinak/gomtg/mtgjson"
)

type State struct {
	Query []string

	Mode       Mode
	PrevMode   Mode
	FilterSet  mtgjson.SetID
	Options    []Card
	Local      []LocalCard
	Sort       Sort
	Tags       []string
	PageOffset int

	Filtered bool

	Selection Selection
	Tagging   []Tagging
	Delete    []LocalCard
}

func (s State) Changes() bool {
	return len(s.Selection) != 0 ||
		len(s.Tagging) != 0 ||
		len(s.Delete) != 0
}

func (s State) SortLocal(app *App) {
	sorter := NewSortable(func(i, j int) {
		s.Local[i], s.Local[j] = s.Local[j], s.Local[i]
	})
	ints, strs := make([]int, 0), make([]string, 0)
	switch s.Sort {
	case SortName:
		for _, c := range s.Local {
			strs = append(strs, c.Name())
		}
	case SortCount:
		for _, c := range s.Local {
			ints = append(ints, app.DB.Count(c.UUID()))
		}
	case SortPrice:
		for _, c := range s.Local {
			p, _ := app.GetPricing(c.UUID(), c.Foil(), false)
			ints = append(ints, int(p*100))
		}
	default:
		for _, c := range s.Local {
			ints = append(ints, c.Index)
		}
	}

	sorter.SetData(ints, strs)
	sorter.Sort()
}

func (s State) SortOptions(app *App) {
	sorter := NewSortable(func(i, j int) {
		s.Options[i], s.Options[j] = s.Options[j], s.Options[i]
	})
	ints, strs := make([]int, 0), make([]string, 0)
	switch s.Sort {
	case SortPrice:
		for _, c := range s.Options {
			p, _ := app.GetPricing(c.UUID, false, false)
			ints = append(ints, int(p*100))
		}
	case SortCount:
		for _, c := range s.Options {
			ints = append(ints, app.DB.Count(c.UUID))
		}
	default:
		for _, c := range s.Options {
			strs = append(strs, c.Name)
		}
	}

	sorter.SetData(ints, strs)
	sorter.Sort()
}

type Sort string

const (
	SortIndex Sort = "index"
	SortName  Sort = "name"
	SortPrice Sort = "price"
	SortCount Sort = "count"
)

var Sorts = map[Sort]struct{}{
	SortIndex: {},
	SortName:  {},
	SortPrice: {},
	SortCount: {},
}

func (s Sort) Valid() bool {
	_, ok := Sorts[s]
	return ok
}

type Selection []Select

func (s Selection) Cards() []Card {
	n := make([]Card, len(s))
	for i, c := range s {
		n[i] = c.Card
	}
	return n
}

type Select struct {
	Card
	Tags newTags
}

func NewSelect(c Card) Select {
	return Select{c, make(newTags)}
}

func NewSelection(c []Card) []Select {
	n := make([]Select, len(c))
	for i, card := range c {
		n[i] = NewSelect(card)
	}
	return n
}

func (s State) Equal(o State) bool {
	if s.Mode != o.Mode ||
		s.PrevMode != o.PrevMode ||
		s.FilterSet != o.FilterSet ||
		len(s.Selection) != len(o.Selection) ||
		len(s.Tags) != len(o.Tags) ||
		len(s.Delete) != len(o.Delete) ||
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

	for i := range s.Tags {
		if s.Tags[i] != o.Tags[i] {
			return false
		}
	}

	for i := range s.Tagging {
		if !s.Tagging[i].Equal(o.Tagging[i]) {
			return false
		}
	}

	for i := range s.Delete {
		if s.Delete[i].DBCard != o.Delete[i].DBCard {
			return false
		}
	}

	return true
}

func (s State) String(app *App) []string {
	data := []string{s.StringShort(app)}
	selCards := make([]Card, len(s.Selection))
	good, bad := app.Colors.Get("good"), app.Colors.Get("bad")
	for i, c := range s.Selection {
		selCards[i] = c.Card
	}
	cardsStrs := app.CardsString(selCards, 0, false)
	for i := range cardsStrs {
		cardsStrs[i] = fmt.Sprintf(" \u2514 %s ADD \033[0m %s", good, cardsStrs[i])
	}
	data = append(data, cardsStrs...)

	tagsAdd := make(map[string]int)
	tagsDel := make(map[string]int)
	for _, c := range s.Tagging {
		for add := range c.tagsAdd {
			tagsAdd[add]++
		}
		for rem := range c.tagsDel {
			tagsDel[rem]++
		}
	}

	for tag, amount := range tagsAdd {
		data = append(
			data,
			fmt.Sprintf(" \u2514 added tag '%s' to %d cards", tag, amount),
		)
	}
	for tag, amount := range tagsDel {
		data = append(
			data,
			fmt.Sprintf(" \u2514 removed tag '%s' from %d cards", tag, amount),
		)
	}

	delStrs := app.LocalCardsString(s.Delete, 0, false)
	for i := range delStrs {
		delStrs[i] = fmt.Sprintf(" \u2514 %s DEL \033[0m %s", bad, delStrs[i])
	}
	data = append(data, delStrs...)

	return data
}

func (s State) StringShort(app *App) string {
	clr := app.Colors.Get("status")
	modeClr := app.Colors.Get("good")
	d := make([]string, 3, 5)
	d[0] = fmt.Sprintf("q:%s", strings.Join(s.Query, " "))
	d[1] = fmt.Sprintf("set:%s", s.FilterSet)
	d[2] = fmt.Sprintf("selected:%d", len(s.Selection))
	if len(s.Local) != 0 {
		outdated := 0
		for _, c := range s.Local {
			_, ok := app.GetPricing(c.UUID(), c.Foil(), false)
			if !ok {
				outdated++
			}
		}
		d = append(d, fmt.Sprintf("bad-prices:%d", outdated))
	}
	if len(s.Options) != 0 {
		d = append(d, fmt.Sprintf("options:%d", len(s.Options)))
	}
	if len(s.Tags) != 0 {
		d = append(d, fmt.Sprintf("tags:%s", strings.Join(s.Tags, ",")))
	}

	mode := fmt.Sprintf("%s %s \033[0m", modeClr, strings.ToUpper(string(s.Mode)))
	return fmt.Sprintf("%s %s %s \033[0m", mode, clr, strings.Join(d, " "))
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

func (t newTags) Add(tags ...string) {
	for _, tag := range tags {
		t[tag] = struct{}{}
	}
}
func (t newTags) Del(tags ...string) {
	for _, tag := range tags {
		delete(t, tag)
	}
}

type Tagging struct {
	*DBCard
	tagsAdd newTags
	tagsDel newTags
}

func NewTagging(c *DBCard) Tagging {
	return Tagging{
		c,
		make(newTags),
		make(newTags),
	}
}

func (t Tagging) Add(add bool, tag string) {
	if add {
		t.tagsAdd.Add(tag)
		t.tagsDel.Del(tag)
		return
	}
	t.tagsDel.Add(tag)
	t.tagsAdd.Del(tag)
}

func (t Tagging) Commit() {
	t.DBCard.Untag(t.tagsDel.Slice())
	t.DBCard.Tag(t.tagsAdd.Slice())
}

func (t Tagging) NewTags() (added []string, removed []string) {
	added, removed = t.tagsAdd.Slice(), t.tagsDel.Slice()
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
	*DBCard
	Index int
}

func NewLocalCard(c *DBCard, ix int) LocalCard {
	return LocalCard{
		DBCard: c,
		Index:  ix,
	}
}
