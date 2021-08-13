package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/frizinak/gomtg/mtgjson"
)

func init() {
	gob.Register(All{})
}

type Card struct {
	UUID          mtgjson.UUID
	Identifiers   mtgjson.ID
	Name          string
	SetCode       mtgjson.SetID
	Availability  mtgjson.Availability
	ColorIdentity mtgjson.Colors
	ManaCost      string
	Keywords      mtgjson.Keywords
	dir           string
}

func (c Card) Full() (mtgjson.Card, error) {
	return mtgjson.ReadCardGOB(c.dir, c.UUID)
}

func (c Card) ImageURLScryfall(back bool, size string) (string, error) {
	if c.Identifiers.ScryfallId == "" {
		return "", errors.New("no scryfall id")
	}
	face := "front"
	if back {
		face = "back"
	}
	if size == "" {
		size = "normal"
	}
	return fmt.Sprintf(
		"https://api.scryfall.com/cards/%s?format=image&face=%s&version=%s",
		c.Identifiers.ScryfallId,
		face,
		size,
	), nil
}

func (c Card) ImageURLGatherer() (string, error) {
	if c.Identifiers.MultiverseId == "" {
		return "", errors.New("no multiverse id")
	}
	return fmt.Sprintf(
		"https://gatherer.wizards.com/Handlers/Image.ashx?type=card&multiverseid=%s",
		c.Identifiers.MultiverseId,
	), nil
}

func (c Card) ImageURL() (string, error) {
	if img, err := c.ImageURLScryfall(false, "normal"); err == nil {
		return img, err
	}

	return c.ImageURLGatherer()
}

type Sets map[mtgjson.SetID]string

type All struct {
	Cards []Card
	Sets  Sets

	uuid map[mtgjson.UUID]int
}

func (a *All) buildByUUID() {
	a.uuid = make(map[mtgjson.UUID]int)
	for i, c := range a.Cards {
		a.uuid[c.UUID] = i
	}
}

func (a *All) ByUUID(uuid mtgjson.UUID) (Card, bool) {
	v, ok := a.uuid[uuid]
	if !ok || v < 0 || v >= len(a.Cards) {
		return Card{}, false
	}
	return a.Cards[v], true
}

func loadData(dir string, refresh bool) (*All, error) {
	file := filepath.Join(dir, "all.gob")
	cardDir := filepath.Join(dir, "cards")
	if !refresh {
		_, err := os.Stat(file)
		if err != nil {
			refresh = true
		}
	}
	if refresh {
		_ = os.MkdirAll(cardDir, 0700)
		destJSON := file + ".json"
		err := progress("Download mtgjson.com data", func() error {
			w, err := os.Create(destJSON)
			if err != nil {
				return err
			}

			if err := mtgjson.DownloadAllPrintings(w); err != nil {
				w.Close()
				return err
			}

			return w.Close()
		})
		if err != nil {
			return nil, err
		}

		all := &All{Cards: make([]Card, 0), Sets: make(Sets)}
		var allCards []mtgjson.Card
		err = progress("Prepare data", func() error {
			in, err := os.Open(destJSON)
			if err != nil {
				return err
			}
			data, err := mtgjson.ReadAllPrintingsJSON(in)
			in.Close()
			if err != nil {
				return err
			}

			data = data.FilterOnlineOnly(false)
			for i := range data {
				all.Sets[i] = data[i].Name
			}

			allCards = data.Cards()
			for _, c := range allCards {
				all.Cards = append(
					all.Cards,
					Card{
						UUID:          c.UUID,
						Identifiers:   c.Identifiers,
						Name:          c.Name,
						SetCode:       c.SetCode,
						Availability:  c.Availability,
						ColorIdentity: c.ColorIdentity,
						ManaCost:      c.ManaCost,
						Keywords:      c.Keywords,
					},
				)
			}

			return nil
		})
		if err != nil {
			return nil, err
		}

		err = progress("Encode to gob", func() error {
			err = mtgjson.WriteCardsGOB(cardDir, allCards)
			if err != nil {
				return err
			}

			tmp := file + ".tmp"
			out, err := os.Create(tmp)
			if err != nil {
				return err
			}

			enc := gob.NewEncoder(out)
			if err = enc.Encode(all); err != nil {
				out.Close()
				return err
			}
			out.Close()
			if err = os.Rename(tmp, file); err != nil {
				return err
			}

			return os.Remove(destJSON)

		})
		if err != nil {
			return nil, err
		}
	}

	all := &All{}
	err := progress("Parse mtgjson.com data", func() error {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		dec := gob.NewDecoder(f)
		return dec.Decode(all)
	})
	if err != nil {
		return nil, err
	}

	for i := range all.Cards {
		all.Cards[i].dir = cardDir
	}

	all.buildByUUID()
	return all, nil
}
