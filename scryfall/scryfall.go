package scryfall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var ErrPleaseWait = errors.New("ratelimited, try again")

const (
	PricingOutdated   = time.Hour * 24
	MaxPerCollections = 75
	deferredIV        = time.Millisecond * 20
	ratelimitIV       = time.Millisecond * 100
)

type cardResult struct {
	Card
	err error
}

type cardRequest struct {
	id  string
	res chan cardResult
}

type Card struct {
	ID              string `json:"id"`
	OracleID        string `json:"oracle_id"`
	PrintsSearchURI string `json:"prints_search_uri"`
	RulingsURI      string `json:"rulings_uri"`
	ScryfallURI     string `json:"scryfall_uri"`
	URI             string `json:"uri"`
	Name            string `json:"name"`
	SetName         string `json:"set_name"`
	Set             string `json:"set"`
	Prices          Prices `json:"prices"`
}

func (c Card) EUR() float64     { return c.Prices.EUR() }
func (c Card) USD() float64     { return c.Prices.USD() }
func (c Card) EURFoil() float64 { return c.Prices.EURFoil() }
func (c Card) USDFoil() float64 { return c.Prices.USDFoil() }

type Prices struct {
	RawEUR     string `json:"eur"`
	RawEURFoil string `json:"eur_foil"`
	RawUSD     string `json:"usd"`
	RawUSDFoil string `json:"usd_foil"`
}

func (p Prices) conv(v string) float64 {
	n, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return 0
	}
	return n
}

func (p Prices) EUR() float64     { return p.conv(p.RawEUR) }
func (p Prices) USD() float64     { return p.conv(p.RawUSD) }
func (p Prices) EURFoil() float64 { return p.conv(p.RawEURFoil) }
func (p Prices) USDFoil() float64 { return p.conv(p.RawUSDFoil) }

type API struct {
	m       sync.Mutex
	running bool

	c       *http.Client
	timeout time.Duration
	rate    chan struct{}

	cardsToFetch chan cardRequest
}

func New(c *http.Client, timeout time.Duration) *API {
	if c == nil {
		c = http.DefaultClient
	}
	if timeout == 0 {
		timeout = time.Second * 30
	}
	return &API{
		c:            c,
		timeout:      timeout,
		rate:         make(chan struct{}, 1),
		cardsToFetch: make(chan cardRequest, 1),
	}
}

func (api *API) Card(id string) (Card, error) {
	var card Card
	api.rate <- struct{}{}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(api.timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.scryfall.com/cards/%s", id), nil)
	if err != nil {
		<-api.rate
		return card, err
	}

	res, err := api.c.Do(req)
	go func() {
		time.Sleep(ratelimitIV)
		<-api.rate
	}()

	if err != nil {
		return card, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		return card, ErrPleaseWait
	}
	if res.StatusCode != http.StatusOK {
		return card, fmt.Errorf("received status code %s", res.Status)
	}

	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&card)

	return card, err
}

func (api *API) CardDeferred(id string) (Card, error) {
	if !api.running {
		api.run()
	}
	ch := make(chan cardResult, 1)
	api.cardsToFetch <- cardRequest{id: id, res: ch}
	res := <-ch
	return res.Card, res.err
}

func (api *API) run() {
	api.m.Lock()
	if api.running {
		api.m.Unlock()
		return
	}
	api.running = true
	api.m.Unlock()
	go func() {
		list := make([]cardRequest, 0, 100)
		do := make(chan struct{}, 1)
		go func() {
			for {
				time.Sleep(time.Second)
				do <- struct{}{}
			}
		}()
		get := func() {
			if len(list) == 0 {
				return
			}
			ids := make([]string, len(list))
			m := make(map[string][]cardRequest, len(list))
			for i, v := range list {
				ids[i] = v.id
				if _, ok := m[v.id]; !ok {
					m[v.id] = make([]cardRequest, 0, 1)
				}
				m[v.id] = append(m[v.id], v)
			}
			res, err := api.Collection(ids)
			list = list[:0]

			for k, v := range res {
				for _, cr := range m[k] {
					cr.res <- cardResult{Card: v}
				}
				delete(m, k)
			}

			if err == nil {
				err = errors.New("not found")
			}
			for _, v := range m {
				for _, cr := range v {
					cr.res <- cardResult{err: err}
				}
			}
		}
		for {
			select {
			case id := <-api.cardsToFetch:
				list = append(list, id)
				if len(list) >= MaxPerCollections {
					get()
				}
			case <-do:
				get()
			case <-time.After(deferredIV):
				get()
			}
		}
	}()
}

type ID struct {
	ID string `json:"id"`
}

type collection struct {
	Identifiers []ID `json:"identifiers"`
}

type collectionResponse struct {
	Data []Card `json:"data"`
}

func (api *API) Collection(ids []string) (map[string]Card, error) {
	var rest []string
	if len(ids) > MaxPerCollections {
		rest = ids[MaxPerCollections:]
		ids = ids[:MaxPerCollections]
	}
	c := collection{make([]ID, len(ids))}
	for i := range ids {
		c.Identifiers[i].ID = ids[i]
	}
	buf := bytes.NewBuffer(nil)
	penc := json.NewEncoder(buf)
	if err := penc.Encode(c); err != nil {
		return nil, err
	}

	api.rate <- struct{}{}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(api.timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.scryfall.com/cards/collection", buf)
	if err != nil {
		<-api.rate
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	res, err := api.c.Do(req)
	go func() {
		time.Sleep(ratelimitIV)
		<-api.rate
	}()

	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, ErrPleaseWait
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status code %s", res.Status)
	}

	col := collectionResponse{}
	dec := json.NewDecoder(res.Body)
	gerr := dec.Decode(&col)

	cmap := make(map[string]Card, len(col.Data))
	for _, c := range col.Data {
		cmap[c.ID] = c
	}
	if len(rest) == 0 {
		return cmap, gerr
	}

	next, err := api.Collection(rest)
	for k, v := range next {
		cmap[k] = v
	}

	if gerr != nil {
		err = gerr
	}

	return cmap, err
}
