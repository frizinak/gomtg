package mtgjson

import "strings"

type SetID string
type BorderColor string
type Color string
type Colors []Color
type FrameEffect string
type Keywords []string
type Layout string
type Rarity string
type UUID string
type Time string
type Availability []string

func (a Availability) Paper() bool {
	for _, v := range a {
		if v == "paper" {
			return true
		}
	}

	return false
}

func (k Keywords) String() string {
	return strings.Join(k, " | ")
}

type LeadershipSkills struct {
	Brawl       bool `json:"brawl"`
	Commander   bool `json:"commander"`
	Oathbreaker bool `json:"oathbreaker"`
}

type Legalities struct {
	Brawl     string `json:"brawl"`
	Commander string `json:"commander"`
	Duel      string `json:"duel"`
	Future    string `json:"future"`
	Frontier  string `json:"frontier"`
	Historic  string `json:"historic"`
	Legacy    string `json:"legacy"`
	Modern    string `json:"modern"`
	Pauper    string `json:"pauper"`
	Penny     string `json:"penny"`
	Pioneer   string `json:"pioneer"`
	Standard  string `json:"standard"`
	Vintage   string `json:"vintage"`
}

type Ruling struct {
	Date Time   `json:"date"`
	Text string `json:"text"`
}

type ForeignData struct {
	FaceName     string `json:"faceName"`
	FlavorText   string `json:"flavorText"`
	Language     string `json:"language"`
	MultiverseID int    `json:"multiverseId"`
	Name         string `json:"name"`
	Text         string `json:"text"`
	Type         string `json:"type"`
}

type ID struct {
	CardKingdomFoilId      string `json:"cardKingdomFoilId"`
	CardKingdomId          string `json:"cardKingdomId"`
	McmId                  string `json:"mcmId"`
	McmMetaId              string `json:"mcmMetaId"`
	MtgArenaId             string `json:"mtgArenaId"`
	MtgoFoilId             string `json:"mtgoFoilId"`
	MtgoId                 string `json:"mtgoId"`
	MtgjsonV4Id            string `json:"mtgjsonV4Id"`
	MultiverseId           string `json:"multiverseId"`
	ScryfallId             string `json:"scryfallId"`
	ScryfallOracleId       string `json:"scryfallOracleId"`
	ScryfallIllustrationId string `json:"scryfallIllustrationId"`
	TcgplayerProductId     string `json:"tcgplayerProductId"`
}

type PurchaseURLs struct {
	CardKingdom     string `json:"cardKingdom"`
	CardKingdomFoil string `json:"cardKingdomFoil"`
	Cardmarket      string `json:"cardmarket"`
	TCGPlayer       string `json:"tcgplayer"`
}
