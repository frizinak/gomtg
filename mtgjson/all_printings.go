package mtgjson

import (
	"errors"
	"fmt"
	"sort"
)

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

type Data struct {
	Type             string `json:"type"`
	ReleaseDate      Time   `json:"releaseDate"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	KeyruneCode      string `json:"keyruneCode"`
	Block            string `json:"block"`
	TCGPlayerGroupId int64  `json:"tcgPlayerGroupId"`
	MCMID            int    `json:"mcmid"`
	MCMName          string `json:"mcmName"`
	TotalSetSize     int    `json:"totalSetSize"`
	BaseSetSize      int    `json:"baseSetSize"`
	IsNonFoilOnly    bool   `json:"isNonFoilOnly"`
	IsFoilOnly       bool   `json:"isFoilOnly"`
	IsOnlineOnly     bool   `json:"isOnlineOnly"`

	Cards []FullCard `json:"cards"`
	// Booster map[string]interface{}
	// Tokens []interface{}
	// SealedProduct
	// Translations
}

type Card struct {
	UUID         UUID         `json:"uuid"`
	Identifiers  ID           `json:"identifiers"`
	Name         string       `json:"name"`
	SetCode      SetID        `json:"setCode"`
	Availability Availability `json:"availability"`
}

type FullCard struct {
	Card
	// Artist                  string        `json:"artist"`
	ASCII                   string        `json:"asciiName"`
	BorderColor             BorderColor   `json:"borderColor"`
	ColorIdentity           Colors        `json:"colorIdentity"`
	ColorIndicator          Colors        `json:"colorIndicator"`
	Colors                  Colors        `json:"colors"`
	ConvertedManaCost       float64       `json:"convertedManaCost"`
	FaceConvertedManaCost   float64       `json:"faceConvertedManaCost"`
	FaceName                string        `json:"faceName"`
	FlavorName              string        `json:"flavorName"`
	FlavorText              string        `json:"flavorText"`
	FrameEffects            []FrameEffect `json:"frameEffects"`
	FrameVersion            string        `json:"frameVersion"`
	Hand                    string        `json:"hand"`
	HasContentWarning       bool          `json:"hasContentWarning"`
	HasFoil                 bool          `json:"hasFoil"`
	HasAlternativeDeckLimit bool          `json:"hasAlternativeDeckLimit"`
	HasNonFoil              bool          `json:"hasNonFoil"`
	IsAlternative           bool          `json:"isAlternative"`
	IsFullArt               bool          `json:"isFullArt"`
	IsOnlineOnly            bool          `json:"isOnlineOnly"`
	IsOversized             bool          `json:"isOversized"`
	IsPromo                 bool          `json:"isPromo"`
	IsReprint               bool          `json:"isReprint"`
	IsReserved              bool          `json:"isReserved"`
	IsStarter               bool          `json:"isStarter"`
	IsStorySpotlight        bool          `json:"isStorySpotlight"`
	IsTextless              bool          `json:"isTextless"`
	IsTimeshifted           bool          `json:"isTimeshifted"`
	Keywords                Keywords      `json:"keywords"`
	Layout                  Layout        `json:"layout"`
	Life                    string        `json:"life"`
	Loyalty                 string        `json:"loyalty"`
	ManaCost                string        `json:"manaCost"`
	Number                  string        `json:"number"`
	OriginalReleaseDate     Time          `json:"originalReleaseDate"`
	OriginalText            string        `json:"originalText"`
	OriginalType            string        `json:"originalType"`
	OtherFaceIds            []UUID        `json:"otherFaceIds"`
	Power                   string        `json:"power"`
	Printings               []SetID       `json:"printings"`
	PromoTypes              []string      `json:"promoTypes"`
	Rarity                  Rarity        `json:"rarity"`
	Side                    string        `json:"side"`
	Subtypes                []string      `json:"subtypes"`
	Supertypes              []string      `json:"supertypes"`
	Text                    string        `json:"text"`
	Toughness               string        `json:"toughness"`
	Type                    string        `json:"type"`
	Types                   []string      `json:"types"`
	Variations              []UUID        `json:"variations"`
	// Watermark               string        `json:"watermark"`
	// PurchaseUrls        `json:"purchaseUrls"`
	// LeadershipSkills        `json:"leadershipSkills"`
	// Legalities              `json:"legalities"`
	// Rulings    `json:"rulings"`
	// EDHRECRank int `json:"edhrecRank"`
	// ForeignData             `json:"foreignData"`
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

type AllPrintings map[SetID]Data

func (p AllPrintings) FilterOnlineOnly(o bool) AllPrintings {
	n := make(AllPrintings)
	for k, pr := range p {
		if pr.IsOnlineOnly == o {
			n[k] = pr
		}
	}
	return n
}

func (p AllPrintings) SetIDs() []SetID {
	n := make([]SetID, 0, len(p))
	for i := range p {
		n = append(n, i)
	}
	sort.Slice(n, func(i, j int) bool {
		return n[i] < n[j]
	})

	return n
}

func (p AllPrintings) FullCards() []FullCard {
	n := make([]FullCard, 0, len(p))
	for i := range p {
		n = append(n, p[i].Cards...)
	}
	return n
}

func (p AllPrintings) Cards() []Card {
	n := make([]Card, 0, len(p))
	for i := range p {
		for j := range p[i].Cards {
			n = append(n, p[i].Cards[j].Card)
		}
	}
	return n
}

func ByUUID(cards []Card) map[UUID]int {
	n := make(map[UUID]int, len(cards))
	for i, c := range cards {
		n[c.UUID] = i
	}

	return n
}
