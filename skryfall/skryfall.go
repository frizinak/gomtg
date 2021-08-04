package skryfall

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const PricingOutdated = time.Hour * 24

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
	c       *http.Client
	timeout time.Duration
}

func New(c *http.Client, timeout time.Duration) *API {
	if c == nil {
		c = http.DefaultClient
	}
	if timeout == 0 {
		timeout = time.Second * 30
	}
	return &API{c, timeout}
}

func (api *API) Card(id string) (Card, error) {
	var card Card

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(api.timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.scryfall.com/cards/%s", id), nil)
	if err != nil {
		return card, err
	}

	res, err := api.c.Do(req)
	if err != nil {
		return card, err
	}
	defer res.Body.Close()

	dec := json.NewDecoder(res.Body)
	if err := dec.Decode(&card); err != nil {
		return card, err
	}

	return card, nil
}
