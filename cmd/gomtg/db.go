package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/frizinak/gomtg/mtgjson"
)

type Card struct {
	Name  string        `json:"name"`
	UUID  mtgjson.UUID  `json:"uuid"`
	SetID mtgjson.SetID `json:"set_id"`
	Tags  []string      `json:"tags"`
}

func FromCard(c mtgjson.Card) *Card {
	return &Card{
		UUID:  c.UUID,
		SetID: c.SetCode,
		Name:  c.Name,
		Tags:  make([]string, 0),
	}
}

func (c *Card) Card(list []mtgjson.Card) (mtgjson.Card, error) {
	for _, crd := range list {
		if c.UUID == crd.UUID {
			return crd, nil
		}
	}

	return mtgjson.Card{}, fmt.Errorf("card with UUID %s not found", c.UUID)
}

type DB struct {
	data []*Card
	save bool
}

func (db *DB) Add(c *Card) {
	db.data = append(db.data, c)
	db.save = true
}

func (db *DB) Cards() []*Card {
	d := make([]*Card, len(db.data))
	copy(d, db.data)
	return d
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
	os.MkdirAll(filepath.Dir(file), 0700)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	for _, c := range db.data {
		if err := enc.Encode(c); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}

	f.Close()
	return os.Rename(tmp, file)
}

func LoadDB(file string) (*DB, error) {
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return &DB{data: make([]*Card, 0)}, nil
		}
		return nil, err
	}
	defer f.Close()

	cards := make([]*Card, 0, 1024)
	dec := json.NewDecoder(f)
	for dec.More() {
		c := &Card{}
		if err := dec.Decode(c); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}

	return &DB{data: cards}, nil
}
