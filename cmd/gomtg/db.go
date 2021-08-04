package main

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/frizinak/gomtg/mtgjson"
)

type Tags map[string]struct{}

func (t Tags) Slice() []string {
	s := make([]string, 0, len(t))
	for i := range t {
		s = append(s, i)
	}
	sort.Strings(s)
	return s
}

func (t Tags) Add(s []string) bool {
	changed := false
	for _, tag := range s {
		if _, ok := t[tag]; !ok {
			changed = true
			t[tag] = struct{}{}
		}
	}
	return changed
}

func (t Tags) Del(s []string) bool {
	changed := false
	for _, tag := range s {
		if _, ok := t[tag]; ok {
			changed = true
			delete(t, tag)
		}
	}

	return changed
}

type Card struct {
	db    *DB
	name  string
	uuid  mtgjson.UUID
	setID mtgjson.SetID
	tags  Tags
	del   bool
}

type jsonCard struct {
	Name  string        `json:"name"`
	UUID  mtgjson.UUID  `json:"uuid"`
	SetID mtgjson.SetID `json:"set_id"`
	Tags  []string      `json:"tags"`
}

func (c *Card) Name() string         { return c.name }
func (c *Card) UUID() mtgjson.UUID   { return c.uuid }
func (c *Card) SetID() mtgjson.SetID { return c.setID }
func (c *Card) Tags() []string       { return c.tags.Slice() }

func (c *Card) Tag(tags []string) {
	changed := c.tags.Add(tags)
	c.db.save = c.db.save || changed
}

func (c *Card) Untag(tags []string) {
	changed := c.tags.Del(tags)
	c.db.save = c.db.save || changed
}

func FromCard(db *DB, c mtgjson.Card) *Card {
	return &Card{
		db:    db,
		uuid:  c.UUID,
		setID: c.SetCode,
		name:  c.Name,
		tags:  make(Tags),
	}
}

type DB struct {
	data   []*Card
	byUUID map[mtgjson.UUID][]int
	save   bool
}

func (db *DB) Add(c *Card) {
	db.data = append(db.data, c)
	if _, ok := db.byUUID[c.uuid]; !ok {
		db.byUUID[c.uuid] = make([]int, 0, 1)
	}
	db.byUUID[c.uuid] = append(db.byUUID[c.uuid], len(db.data)-1)
	db.save = true
}

func (db *DB) AddMTGJSON(c mtgjson.Card) {
	db.Add(FromCard(db, c))
}

func (db *DB) Cards() []*Card {
	d := make([]*Card, len(db.data))
	copy(d, db.data)
	return d
}

func (db *DB) Count(uuid mtgjson.UUID) int {
	return len(db.byUUID[uuid])
}

func (db *DB) CardAt(ix int) (*Card, bool) {
	if ix < 0 || ix >= len(db.data) {
		return nil, false
	}
	return db.data[ix], true
}

func (db *DB) Save(file string) error {
	if !db.save {
		_, err := os.Stat(file)
		if err == nil {
			return nil
		}
	}

	tmp := file + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	for _, c := range db.data {
		jc := jsonCard{
			c.name,
			c.uuid,
			c.setID,
			c.Tags(),
		}
		if err := enc.Encode(jc); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}

	f.Close()
	return os.Rename(tmp, file)
}

func LoadDB(file string) (*DB, error) {
	byUUID := make(map[mtgjson.UUID][]int)
	db := &DB{data: make([]*Card, 0, 1024), byUUID: byUUID}
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return db, nil
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for dec.More() {
		jc := &jsonCard{}
		if err := dec.Decode(jc); err != nil {
			return nil, err
		}
		tags := make(Tags)
		tags.Add(jc.Tags)
		c := &Card{
			db,
			jc.Name,
			jc.UUID,
			jc.SetID,
			tags,
			false,
		}
		db.Add(c)
	}
	db.save = false

	return db, nil
}
