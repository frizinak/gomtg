package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/frizinak/gomtg/fuzzy"
	"github.com/frizinak/gomtg/mtgjson"
	"github.com/frizinak/gomtg/skryfall"
	"github.com/mattn/go-runewidth"
	"github.com/nightlyone/lockfile"
)

func progress(msg string, cb func() error) error {
	fmt.Printf("\033[?25l[ ] %s", msg)
	ts := time.Now()
	if err := cb(); err != nil {
		fmt.Println()
		return err
	}
	fmt.Printf("\033[2GX\033[30C %dms\n\033[?25h", int(time.Since(ts).Milliseconds()))
	return nil
}

func intRange(str string) ([]int, bool) {
	comma := strings.FieldsFunc(str, func(r rune) bool {
		return r == ',' || r == ' '
	})
	r := make([]int, 0, len(comma))
	for _, n := range comma {
		dash := strings.SplitN(n, "-", 2)
		v, err := strconv.Atoi(strings.TrimSpace(dash[0]))
		if err != nil {
			return r, false
		}
		if len(dash) != 2 {
			r = append(r, v)
			continue
		}

		if len(dash) == 2 {
			v2, err := strconv.Atoi(strings.TrimSpace(dash[1]))
			if err != nil {
				return r, false
			}
			if v2 < v {
				return r, false
			}
			for i := v; i <= v2; i++ {
				r = append(r, i)
			}
		}
	}

	return r, len(r) > 0
}

func uniqUUIDPart(list []string) [][3]string {
	if len(list) > 10000 {
		data := make([][3]string, len(list))
		for i := range list {
			data[i][1] = list[i]
		}
		return data
	}

	data := make(map[string]map[int]struct{}, len(list))
	for i, item := range list {
		for ln := 1; ln <= len(item); ln++ {
			for n := 0; n <= len(item)-ln; n++ {
				sub := item[n : n+ln]
				if strings.Contains(sub, "-") {
					continue
				}
				if _, ok := data[sub]; !ok {
					data[sub] = make(map[int]struct{}, 1)
				}
				data[sub][i] = struct{}{}
			}
		}
	}

	ret := make([]string, len(list))
	wasAlpha := make([]bool, len(list))
	for i, s := range data {
		if len(s) == 0 {
			continue
		}

		if len(s) != 1 {
			value := ""
			identical := true
			for ix := range s {
				if value == "" {
					value = list[ix]
				}
				if list[ix] != value {
					identical = false
					break
				}
			}
			if !identical {
				continue
			}
		}

		for k := range s {
			alpha := true
			for _, c := range i {
				if c < 'a' || c > 'z' {
					alpha = false
					break
				}
			}
			switch {
			case len(ret[k]) == 0,
				alpha == wasAlpha[k] && len(i) < len(ret[k]),
				alpha == wasAlpha[k] && len(i) == len(ret[k]) && i < ret[k],
				!alpha && wasAlpha[k] && len(i)+4 < len(ret[k]),
				alpha && !wasAlpha[k] && len(i) < len(ret[k])+4:
				wasAlpha[k] = alpha
				ret[k] = i
			}
		}
	}

	d := make([][3]string, len(list))
	for i := range ret {
		if ret[i] == "" {
			d[i][1] = list[i]
			continue
		}
		ix := strings.Index(list[i], ret[i])
		d[i][0] = list[i][0:ix]
		d[i][1] = list[i][ix : ix+len(ret[i])]
		d[i][2] = list[i][ix+len(ret[i]):]
	}

	return d
}

func colorUniqUUID(uuids []string, colors Colors) []string {
	list := uniqUUIDPart(uuids)
	ret := make([]string, len(uuids))
	clrH, clrL := "", ""
	if colors != nil {
		clrH = colors.Get("high")
		clrL = colors.Get("low")
	}
	for i := range list {
		ret[i] = fmt.Sprintf(
			"%s%s\033[0m%s %s \033[0m%s%s\033[0m",
			clrL,
			list[i][0],
			clrH,
			list[i][1],
			clrL,
			list[i][2],
		)
	}

	return ret
}

type getPricing func(uuid mtgjson.UUID, fetch bool) (float64, bool)

func cardsString(db *DB, cards []mtgjson.Card, max int, getPricing getPricing, colors Colors, uniq bool) []string {
	l := make([]string, 0, len(cards))
	uuids := make([]string, len(cards))
	for i, c := range cards {
		uuids[i] = string(c.UUID)
	}

	if uniq {
		uuids = colorUniqUUID(uuids, colors)
	}

	longestTitle := 0
	for _, c := range cards {
		l := len(c.Name)
		if l > longestTitle {
			longestTitle = l
		}
	}
	titlePad := strconv.Itoa(longestTitle)
	bad := colors.Get("bad")

	if max > 0 && len(cards) > max {
		s := len(cards) - max
		uuids = uuids[s:]
		cards = cards[s:]
	}

	for i, c := range cards {
		pricing, ok := getPricing(cards[i].UUID, false)
		pricingClr := ""
		if !ok {
			pricingClr = bad
		}
		l = append(
			l,
			fmt.Sprintf(
				"%s \u2502 %-5s \u2502 %-4d \u2502 %-"+titlePad+"s \u2502%s %-.2f \033[0m",
				uuids[i],
				c.SetCode,
				db.Count(c.UUID),
				c.Name,
				pricingClr,
				pricing,
			),
		)
	}

	return l
}

var csiRE = regexp.MustCompile(`\033\[.*?[a-z]`)

func localCardsString(db *DB, cards []LocalCard, max int, getPricing getPricing, colors Colors, uniq bool) []string {
	if len(cards) == 0 {
		return nil
	}
	l := make([]string, 0, len(cards)+1)
	uuids := make([]string, len(cards))
	for i, c := range cards {
		uuids[i] = string(c.UUID())
	}

	if uniq {
		uuids = colorUniqUUID(uuids, colors)
	}

	longestTitle := 0
	for _, c := range cards {
		l := len(c.Name())
		if l > longestTitle {
			longestTitle = l
		}
	}
	titlePad := strconv.Itoa(longestTitle)
	bad := colors.Get("bad")

	priceSum := 0.0
	priceFails := len(cards)
	prices := make([][]byte, len(cards))
	//longestPrice := 0
	for i := range prices {
		pricing, ok := getPricing(cards[i].UUID(), false)
		p := fmt.Sprintf("%.2f", pricing)
		if pricing == 0 {
			ok = false
			p = "-"
		}
		b := make([]byte, len(p)+1)
		copy(b[1:], p)
		if ok {
			b[0] = 1
			priceFails--
			priceSum += pricing
		}
		prices[i] = b
	}
	priceSumStr := fmt.Sprintf("%.2f", priceSum)
	pricePad := strconv.Itoa(len(priceSumStr))

	p1 := "%6d \u2502 %s \u2502 %-5s \u2502 %-4d \u2502 %-" +
		titlePad + "s "
	p2 := "\u2502%s %" + pricePad + "s \033[0m\u2502 %s"

	p1Len := 0
	if max > 0 && len(cards) > max {
		s := len(cards) - max
		uuids = uuids[s:]
		prices = prices[s:]
		cards = cards[s:]
	}
	for i, c := range cards {
		tags := c.Tags()
		tagstr := strings.Join(tags, ",")
		pricingClr := ""
		if prices[i][0] == 0 {
			pricingClr = bad
		}
		items := []string{
			fmt.Sprintf(
				p1,
				c.Index+1,
				uuids[i],
				c.SetID(),
				db.Count(c.UUID()),
				c.Name(),
			),
			fmt.Sprintf(
				p2,
				pricingClr,
				prices[i][1:],
				tagstr,
			),
		}
		if p1Len == 0 {
			p1Len = runewidth.StringWidth(csiRE.ReplaceAllString(items[0], ""))
		}

		l = append(l, strings.Join(items, ""))
	}

	pricingClr := colors.Get("good")
	if priceFails != 0 {
		pricingClr = bad
	}
	formatTotal := "%" + strconv.Itoa(p1Len) + "s\u2502%s %.2f \033[0m\u2502"
	l = append(l, fmt.Sprintf(formatTotal, "", pricingClr, priceSum))

	return l
}

func cardListID(c []mtgjson.Card) string {
	ids := make([]string, len(c))
	for i, card := range c {
		ids[i] = string(card.UUID)
	}

	return strings.Join(ids, " ")
}

func main() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get your cache directory: %s\n", err)
		os.Exit(0)
	}
	dir := filepath.Join(cacheDir, "gomtg")
	dest := filepath.Join(dir, "v5-all-printings.gob")
	imagePath := filepath.Join(dir, "options.jpg")
	imageDir := filepath.Join(dir, "images")
	var skipIntro bool
	var imageCommand string
	var imageAutoReload bool
	var imageRefreshCommand string
	var imageAutoView bool
	var imageNoCache bool
	var dbFile string
	var noPricing bool
	var currency string
	colors := Colors{
		"bad":    {0, 2, 1},
		"good":   {1, 3, 1},
		"high":   {8, 1, 1},
		"low":    {0, 1, 0},
		"status": {3, 1, 0},
	}
	colorStr := colors.Encode()
	var testColors bool

	flag.BoolVar(&skipIntro, "n", false, "Skip intro")
	flag.StringVar(
		&imageCommand,
		"i",
		"",
		`Command to run to view images. {fn} will be replaced by the filename.
if no {fn} argument is found, it will be appended to the command.
e.g.: -i "chromium 'file://{fn}'",
e.g.: -i 'imv' -ia,
e.g.: -i 'imv' -ir '/bin/sh -c "imv-msg {pid} close all; imv-msg {pid} open {fn}"'`,
	)
	flag.BoolVar(
		&imageAutoReload,
		"ia",
		false,
		`Your image viewer wont be killed and respawned each time you view an image.
Only useful if your viewer auto reloads updated images (imv and feh for example)`,
	)
	flag.StringVar(
		&imageRefreshCommand,
		"ir",
		"",
		`command run each time the image should be refreshed.
ignored if -ia is passed. {fn} is replaced by the filename and {pid} with the process id.`,
	)
	flag.BoolVar(&imageNoCache, "no-cache", false, fmt.Sprintf("disable image caching (in '%s')", imageDir))
	flag.StringVar(&dbFile, "db", "gomtg.db", "Database file to use")
	flag.StringVar(&colorStr, "c", colorStr, "change default colors (key:bg:fg:bold[,key:value...])")
	flag.BoolVar(&testColors, "color-test", testColors, "test colors")
	flag.BoolVar(&imageAutoView, "iav", false, "Show last added card in image viewer")
	flag.BoolVar(&noPricing, "np", false, "Disable automatically pricing newly added cards")
	flag.StringVar(&currency, "currency", "EUR", "EUR or USD")
	flag.Parse()

	currency = strings.ToLower(currency)
	if currency != "eur" && currency != "usd" {
		fmt.Fprintln(os.Stderr, "invalid currency")
		os.Exit(0)
	}

	_ = os.MkdirAll(dir, 0700)
	_ = os.MkdirAll(filepath.Dir(dbFile), 0700)
	_ = os.MkdirAll(imageDir, 0700)

	imageGetter := func(url string) (image.Image, error) {
		return getImageCached(url, imageDir)
	}
	if imageNoCache {
		imageGetter = func(url string) (image.Image, error) {
			return getImage(url)
		}
	}

	lockFile, err := filepath.Abs(dbFile + ".lock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get absolute path to %s", dbFile)
		os.Exit(1)
	}
	locker, err := lockfile.New(lockFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lockfile error: %s", err.Error())
		os.Exit(1)
	}

	skry := skryfall.New(nil, time.Second*10)
	pricing := make(map[mtgjson.UUID]Pricing)
	pricingBusy := make(map[mtgjson.UUID]struct{})
	var pricingMutex sync.RWMutex
	pricingValue := func(p Pricing) float64 { return p.EUR }
	if currency != "eur" {
		pricingValue = func(p Pricing) float64 { return p.USD }
	}

	sigCh := make(chan os.Signal, 1)
	cancelCh := make(chan struct{}, 1)
	signal.Notify(
		sigCh,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	cleanup := func() {
		fmt.Println("\033[?25h")
		killViewer()
		_ = locker.Unlock()
	}
	bye := func() {
		cleanup()
		os.Exit(0)
	}
	exit := func(err error) {
		if err == nil {
			return
		}
		fmt.Fprintln(os.Stderr, err.Error())
		cleanup()
		os.Exit(1)
	}

	ncolors, err := DecodeColors(colorStr)
	exit(err)
	colors = ncolors.Merge(colors)
	if testColors {
		d := map[string]string{
			"bad":    "an error",
			"good":   "an important message",
			"high":   "highlighted",
			"low":    "nobody cares about this",
			"status": "mode:test set: selected:>3000",
		}
		for k, v := range d {
			fmt.Printf("%s %s \033[0m\n", colors.Get(k), v)
		}
		os.Exit(0)
	}

	go func() {
		for s := range sigCh {
			if s == syscall.SIGINT {
				go func() { cancelCh <- struct{}{} }()
				continue
			}

			bye()
		}
	}()

	exit(progress("Lock database", func() error {
		try := 0
		for {
			try++
			if err := locker.TryLock(); err != nil {
				if err != lockfile.ErrNotExist {
					return fmt.Errorf("Failed to get lock: %w", err)
				}
				time.Sleep(time.Millisecond * 50)
				if try > 20 {
					return errors.New("Could not claim lock")
				}
				continue
			}
			break
		}
		return nil
	}))

	var db *DB
	var localFuzz *fuzzy.Index

	rebuildLocalFuzz := func() {
		list := make([]string, 0)
		for _, card := range db.Cards() {
			list = append(list, card.Name())
		}
		localFuzz = fuzzy.NewIndex(2, list)
	}

	exit(progress("Load database", func() error {
		var err error
		db, err = LoadDB(dbFile)
		pricingMutex.Lock()
		for _, card := range db.Cards() {
			p := card.Pricing()
			if p.T != (time.Time{}) {
				pricing[card.UUID()] = p
			}
		}
		pricingMutex.Unlock()
		return err
	}))

	exit(progress("Create local index", func() error {
		rebuildLocalFuzz()
		return nil
	}))

	data, err := loadData(dest)
	exit(err)

	var cards []mtgjson.Card
	var byUUID map[mtgjson.UUID]int
	exit(progress("Filter paper cards", func() error {
		data = data.FilterOnlineOnly(false)
		cards = data.Cards()
		byUUID = mtgjson.ByUUID(cards)
		return nil
	}))

	sets := make(map[mtgjson.SetID]struct{})
	for i := range data {
		sets[i] = struct{}{}
	}

	var fuzz *fuzzy.Index
	exit(progress("Create full index", func() error {
		list := make([]string, 0)
		for _, card := range cards {
			list = append(list, card.Name)
		}
		fuzz = fuzzy.NewIndex(2, list)
		return nil
	}))

	state := State{Mode: ModeCollection, Sort: SortIndex}
	output := make([]string, 1, 30)

	print := func(msg ...string) {
		output = append(output, msg...)
	}
	printDiv := func() {
		max := 0
		for _, o := range output {
			l := runewidth.StringWidth(csiRE.ReplaceAllString(o, ""))
			if l > max {
				max = l
			}
		}

		n := make([]rune, max)
		for i := range n {
			n[i] = '_'
		}
		print(string(n))
	}
	printAlert := func(msg string) {
		clr := colors.Get("good")
		print(fmt.Sprintf("%s %s \033[0m", clr, msg))
	}
	printErr := func(err error) {
		if err == nil {
			return
		}

		clr := colors.Get("bad")
		print(fmt.Sprintf("%s%s\033[0m", clr, err.Error()))
	}
	flush := func() {
		fmt.Print("\033[2J\033[0;0H")
		output[0] = state.StringShort(colors) + "\n"
		fmt.Print(strings.Join(output, "\n"))
		output = make([]string, 1, 30)
	}

	queue := []State{state}

	modifyState := func(undoable bool, cb func(s State) State) {
		ostate := state
		state = cb(state)
		if state.PrevMode != ostate.Mode && ostate.Mode.ValidInput() {
			state.PrevMode = ostate.Mode
			state.PageOffset = 0
		}
		if undoable && !ostate.Equal(state) {
			queue = append(queue, state)
		}
	}

	lastImageListID := ""
	printSets := func(filter string) {
		filter = strings.ToLower(filter)
		list := make([]string, 0, len(data))
		for _, d := range data {
			v := fmt.Sprintf("%s: %s", d.Code, d.Name)
			if filter != "" && !strings.Contains(strings.ToLower(v), filter) {
				continue
			}
			list = append(list, v)
		}
		sort.Strings(list)
		for _, item := range list {
			print(item)
		}
	}

	cardByUUID := func(uuid mtgjson.UUID) (card mtgjson.Card, ok bool) {
		var ix int
		ix, ok = byUUID[uuid]
		if !ok {
			return
		}
		if ix < 0 || ix >= len(cards) {
			ok = false
			return
		}

		card = cards[ix]
		return
	}

	getFullPricing := func(uuid mtgjson.UUID, fetch bool, wait bool) Pricing {
		p := Pricing{T: time.Now()}
		c, ok := cardByUUID(uuid)
		if !ok || c.Identifiers.ScryfallId == "" {
			return p
		}
		id := c.Identifiers.ScryfallId
		check := func() (Pricing, bool) {
			v, ok := pricing[uuid]
			if ok {
				pv := pricingValue(v)
				if pv != 0 && time.Since(v.T) < skryfall.PricingOutdated {
					return v, true
				}
				if pv == 0 && time.Since(v.T) < time.Minute*5 {
					return v, true
				}
			}
			return p, false
		}

		pricingMutex.RLock()
		v, ok := check()
		pricingMutex.RUnlock()
		if ok {
			return v
		}

		if !fetch {
			return p
		}

		w := make(chan struct{}, 1)
		go func() {
			defer func() { w <- struct{}{} }()

			fetch := func() bool {
				pricingMutex.Lock()
				defer pricingMutex.Unlock()
				if _, ok := pricingBusy[uuid]; ok {
					return false
				}
				if _, ok := check(); ok {
					return false
				}
				pricingBusy[uuid] = struct{}{}
				return true
			}()

			if !fetch {
				return
			}

			res, err := skry.Card(id)
			pricingMutex.Lock()
			defer pricingMutex.Unlock()
			delete(pricingBusy, uuid)
			if err != nil {
				pricing[uuid] = p
				return
			}

			p.EUR = res.EUR()
			p.USD = res.USD()
			p.EURFoil = res.EURFoil()
			p.USDFoil = res.USDFoil()

			pricing[uuid] = p
		}()
		if wait {
			<-w
		}

		return p
	}

	getPricing := func(uuid mtgjson.UUID, fetch bool) (float64, bool) {
		p := getFullPricing(uuid, fetch, false)
		v := pricingValue(p)
		return v, v != 0 && time.Since(p.T) <= skryfall.PricingOutdated
	}

	printSkipped := func(n, max int) {
		if max == 0 || n <= max {
			return
		}
		printAlert(
			fmt.Sprintf(
				"Skipped %d cards, showing last %d",
				n-max,
				max,
			),
		)
	}

	printOptions := func(max int) {
		if state.Mode == ModeCollection {
			state.SortLocal(getPricing)
			print(localCardsString(db, state.Local, max, getPricing, colors, true)...)
			printSkipped(len(state.Local), max)
			return
		}
		state.SortOptions(getPricing)
		print(cardsString(db, state.Options, max, getPricing, colors, true)...)
		printSkipped(len(state.Options), max)
	}

	lastAdded := make([]mtgjson.Card, 0)
	addToCollection := func(cards []mtgjson.Card) {
		if len(cards) == 0 {
			return
		}
		modifyState(true, func(s State) State {
			sel := NewSelection(cards)
			for i := range sel {
				sel[i].Tags.Add(state.Tags...)
			}
			s.Selection = append(s.Selection, sel...)
			return s
		})

		for i := range cards {
			getPricing(cards[i].UUID, !noPricing)
			times := 1
			cut := len(lastAdded)
			for j := len(lastAdded) - 1; j >= 0; j-- {
				if lastAdded[j].UUID == cards[i].UUID {
					times++
					cut = j
				}
			}
			lastAdded = lastAdded[cut:]
			lastAdded = append(lastAdded, cards[i])
			printAlert(
				fmt.Sprintf(
					"Added '%s' to selection (%d times)",
					cards[i].Name,
					times,
				),
			)
		}

		err := genImages(cards, imagePath, imageGetter, func(i, total int) {})
		if err != nil {
			printErr(err)
			return
		}
		lastImageListID = ""
		err = spawnViewer(imageCommand, imageRefreshCommand, imageAutoReload, imagePath)
		if err != nil {
			printErr(err)
		}
	}

	partialUUID := func(str string) (mtgjson.Card, error) {
		list := make([]mtgjson.Card, 0, 1)
		add := func(c mtgjson.Card) error {
			if !strings.Contains(
				strings.ToLower(string(c.UUID)),
				strings.ToLower(str),
			) {
				return nil
			}

			identical := true
			var value mtgjson.UUID
			for _, c := range list {
				if value == "" {
					value = c.UUID
				}
				if c.UUID != value {
					identical = false
					break
				}
			}

			if len(list) > 0 {
				if !identical {
					return errors.New("multiple cards match")
				}
				return nil
			}
			list = append(list, c)
			return nil
		}

		var match mtgjson.Card
		for _, c := range state.Options {
			if err := add(c); err != nil {
				return match, err
			}
		}

		if len(list) == 0 {
			for _, c := range cards {
				if err := add(c); err != nil {
					return match, err
				}
			}
		}

		if len(list) == 0 {
			return match, errors.New("no such card")
		}

		return list[0], nil
	}

	_commandQ := func([]string) error {
		if len(queue) == 1 {
			print("Queue is empty")
			return nil
		}
		for _, s := range queue[1:] {
			print("- " + s.String(db, colors, getPricing))
		}
		return nil
	}

	refreshCh := make(chan struct{}, 1)
	refresh := func() { go func() { refreshCh <- struct{}{} }() }

	commands := map[string]func(arg []string) error{
		"help": func([]string) error {
			print("Usage:")
			print("SIGINT (Ctrl-c)               cancel action in progress")
			print("/help                         this")
			print("/exit   | /quit               quit")
			print("/queue  | /q                  view operation queue")
			print("/sets <filter>                print all known sets (optionally filtered)")
			print("/sort <sort>                  sort items by index, name or price")
			print("/undo   | /u                  remove last item from queue")
			print("/images | /imgs               create a collage of all cards in current list")
			print("/image  | /img <uuid>         show card image for card with (partial) UUID <uuid>")
			print("/prices                       refresh pricing data (async) for cards in collection")
			print("/price <uuid>                 show pricing for card with (partial) UUID")
			print("/tag  {+|-}<tag>,…            tag/untag cards in collection with <tag> or tag all future cards added with <tag>")
			print("                                - mode:collection: filter your collection (/mode collection)")
			print("                                                   and add / remove tags")
			print("                                - mode:add:        set tags to be added for each card added to your collection")
			print("                                                   -<tag> does nothing")
			print("                              e.g.: +nm -played +shoebox")
			print("/commit                       commit selection to file (empties selection)")
			print("/mode   | /m <mode>           enter <mode>")
			print("                                - add:           add cards by entering their name (fuzzy)")
			print("                                - collection:    search your collection for cards")
			print("                                                 by name (fuzzy) or a range (1,2,8-10)")
			print("                                                 filter by tag with +<tag> to only include items with <tag>")
			print("                                                                    -<tag> to exclude items with <tag>")
			print("                                - search:        search all cards (fuzzy)")
			print("/repeat | /r                  add last card again")
			print("/set    | /s <set>            only operate on cards within the given set")
			return nil
		},
		"exit": func([]string) error {
			bye()
			return nil
		},
		"quit": func([]string) error {
			bye()
			return nil
		},
		"queue": _commandQ,
		"undo": func([]string) error {
			if len(queue) > 1 {
				queue = queue[:len(queue)-1]
			}
			state = queue[len(queue)-1]
			return _commandQ(nil)
		},
		"commit": func([]string) error {
			selection := state.Selection
			state.Selection = nil
			for i := range queue {
				queue[i].Selection = nil
			}

			for _, c := range selection {
				dbCard := FromCard(db, c.Card)
				dbCard.Tag(c.Tags.Slice())
				db.Add(dbCard)
			}

			for _, c := range db.Cards() {
				c.SetPricing(getFullPricing(c.UUID(), false, false))
			}

			tags := state.Tagging
			state.Tagging = nil
			for i := range queue {
				queue[i].Tagging = nil
			}
			for _, t := range tags {
				t.Commit()
			}

			saved, err := db.Save(dbFile)
			if err != nil {
				return err
			}

			rebuildLocalFuzz()
			if !saved {
				printErr(errors.New("nothing to commit"))
				return nil
			}
			printAlert("all changes committed to database")
			return nil
		},
		"sets": func(args []string) error {
			printSets(strings.Join(args, " "))
			return nil
		},
		"images": func([]string) error {
			if len(state.Options) > 100 {
				return errors.New("too many cards to generate an image of")
			}
			listID := cardListID(state.Options)
			if lastImageListID == listID {
				printOptions(0)
				return spawnViewer(imageCommand, imageRefreshCommand, imageAutoReload, imagePath)
			}
			err := genImages(state.Options, imagePath, imageGetter, func(i, total int) {
				print(fmt.Sprintf("Downloaded %02d/%02d", i, total))
				flush()
			})
			if err != nil {
				return err
			}
			lastImageListID = listID
			printOptions(0)
			printAlert(fmt.Sprintf("Downloaded image to '%s'", imagePath))
			return spawnViewer(imageCommand, imageRefreshCommand, imageAutoReload, imagePath)
		},
		"image": func(a []string) error {
			if len(a) != 1 || len(a[0]) == 0 {
				return errors.New("/img requires exactly 1 argument")
			}

			list := make([]mtgjson.Card, 1)
			arg := a[0]
			card, err := partialUUID(arg)
			if err != nil {
				return err
			}
			list[0] = card

			listID := cardListID(list)
			if lastImageListID != listID {
				err := genImages(list, imagePath, imageGetter, func(n, total int) {})
				if err != nil {
					return err
				}
				lastImageListID = listID
			}

			printOptions(0)
			printAlert(fmt.Sprintf("Downloaded image to '%s'", imagePath))
			return spawnViewer(imageCommand, imageRefreshCommand, imageAutoReload, imagePath)
		},
		"mode": func(args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			m := Mode(arg)
			c := 0
			if !m.ValidInput() {
				for mode := range AllInputModes {
					if strings.HasPrefix(string(mode), arg) {
						c++
						m = mode
					}
				}
			}
			if c > 1 || !m.ValidInput() {
				return fmt.Errorf("invalid mode: '%s'", arg)
			}

			modifyState(true, func(s State) State {
				if s.Mode != m {
					s.Local = nil
					s.Options = nil
				}
				s.Mode = m
				return s
			})
			refresh()

			return nil
		},
		"set": func(args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			if arg == "" {
				modifyState(true, func(s State) State {
					s.FilterSet = ""
					return s
				})
				return nil
			}
			fs := mtgjson.SetID(strings.ToUpper(arg))
			if _, ok := sets[fs]; !ok {
				printSets("")
				return fmt.Errorf("invalid set id: '%s'", arg)
			}
			modifyState(true, func(s State) State {
				s.FilterSet = fs
				return s
			})
			return nil
		},
		"sort": func(args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			if arg == "" {
				modifyState(true, func(s State) State {
					s.Sort = SortIndex
					return s
				})
				printOptions(0)
				return nil
			}
			sorting := Sort(arg)
			if !sorting.Valid() {
				opts := make([]string, 0, len(Sorts))
				for i := range Sorts {
					opts = append(opts, string(i))
				}
				sort.Strings(opts)
				return fmt.Errorf("'%s' is not a valid sort [%s]", arg, strings.Join(opts, ", "))
			}
			modifyState(true, func(s State) State {
				s.Sort = sorting
				return s
			})

			printOptions(0)
			return nil
		},
		"repeat": func([]string) error {
			if len(state.Selection) == 0 {
				return errors.New("nothing to repeat")
			}
			addToCollection(state.Selection[len(state.Selection)-1:].Cards())

			return nil
		},
		"prices": func([]string) error {
			if state.Mode != ModeCollection {
				return errors.New("/prices can only be called from /mode collection")
			}

			for _, c := range state.Local {
				getPricing(c.UUID(), true)
			}

			return nil
		},
		"price": func(a []string) error {
			if len(a) != 1 || len(a[0]) == 0 {
				return errors.New("/price requires exactly 1 argument")
			}
			card, err := partialUUID(a[0])
			if err != nil {
				return err
			}

			o := getFullPricing(card.UUID, false, true)
			n := getFullPricing(card.UUID, true, true)
			_, ok := getPricing(card.UUID, false)
			if !ok {
				return errors.New("failed to fetch price")
			}
			if n.T == o.T {
				printAlert("price already up to date")
				return nil
			}
			printAlert("price updated")

			return nil
		},
		"tag": func(args []string) error {
			if len(args) == 0 && state.Mode != ModeCollection {
				modifyState(true, func(s State) State {
					s.Tags = nil
					return s
				})
			}

			if state.Mode != ModeCollection && state.Mode != ModeAdd {
				return errors.New("/tags can only be called from /mode collection or /mode add")
			}

			switch state.Mode {
			case ModeCollection:
				tags := make([]Tagging, 0, len(args))
				for _, arg := range args {
					if len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
						return fmt.Errorf("'%s' is no a valid tag specifier", arg)
					}
					for _, c := range state.Local {
						t := NewTagging(c.Card)
						t.Add(arg[0] == '+', arg[1:])
						tags = append(tags, t)
					}
				}

				if len(tags) == 0 {
					return nil
				}

				modifyState(true, func(s State) State {
					s.Tagging = append(s.Tagging, tags...)
					return s
				})

				printAlert(fmt.Sprintf("Updated %d card(s)", len(state.Local)))
			case ModeAdd:
				tags := make([]string, 0, len(args))
				for _, arg := range args {
					if len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
						return fmt.Errorf("'%s' is no a valid tag specifier", arg)
					}
					if arg[0] == '+' {
						tags = append(tags, arg[1:])
					}
				}
				modifyState(true, func(s State) State {
					s.Tags = tags
					return s
				})
			}

			return nil
		},
	}

	commands["quit"] = commands["exit"]
	commands["img"] = commands["image"]
	commands["imgs"] = commands["images"]
	commands["u"] = commands["undo"]
	commands["m"] = commands["mode"]
	commands["s"] = commands["set"]
	commands["q"] = commands["queue"]
	commands["r"] = commands["repeat"]

	var handleCommand func(f []string) (bool, error)
	handleCommand = func(f []string) (bool, error) {
		isCommand := false
		if len(f) == 0 {
			return false, nil
		}

		if strings.HasPrefix(f[0], "/") {
			d := strings.TrimLeft(f[0], "/")
			if cmd, ok := commands[d]; ok {
				return true, cmd(f[1:])
			}

			handleCommand([]string{"/help"})
			print("")
			return true, fmt.Errorf("no such command: '/%s'", d)
		}

		return isCommand, nil
	}

	searchAll := func(qry string) []mtgjson.Card {
		res := fuzz.Search(qry, func(score, min, max int) bool {
			return score > 0 && score == max
		})

		list := make([]mtgjson.Card, 0, len(res))
		for _, ix := range res {
			if state.FilterSet != "" && cards[ix].SetCode != state.FilterSet {
				continue
			}
			list = append(list, cards[ix])
		}

		return list
	}

	numericRE := regexp.MustCompile(`^\d[\d,\- ]+$`)
	searchLocal := func(qry []string) ([]LocalCard, error) {
		qryTags := make([]string, 0, len(qry))
		qryNotTags := make([]string, 0, len(qry))
		_qryStr := make([]string, 0, len(qry))
		for _, p := range qry {
			switch {
			case len(p) == 0:
				continue
			case p[0] == '+':
				qryTags = append(qryTags, p[1:])
			case p[0] == '-':
				qryNotTags = append(qryNotTags, p[1:])
			default:
				_qryStr = append(_qryStr, p)
			}
		}
		qryStr := strings.Join(_qryStr, " ")

		noTags := len(qryTags) == 0 && len(qryNotTags) == 0

		filter := func(c *Card) bool {
			return state.FilterSet == "" || c.SetID() == state.FilterSet
		}

		if !noTags {
			ofilter := filter
			filter = func(c *Card) bool {
				if !ofilter(c) {
					return false
				}
				for _, t := range qryTags {
					if !c.HasTag(t) {
						return false
					}
				}
				for _, t := range qryNotTags {
					if c.HasTag(t) {
						return false
					}
				}
				return true
			}
		}

		search := func() []int {
			return localFuzz.Search(qryStr, func(score, min, max int) bool {
				return score > 0 && score == max
			})
		}

		if qryStr == "" {
			search = func() []int {
				all := db.Cards()
				list := make([]int, 0, len(all))
				for i := range all {
					list = append(list, i)
				}
				return list
			}
		} else if numericRE.MatchString(qryStr) {
			ints, ok := intRange(qryStr)
			if ok {
				search = func() []int {
					res := make([]int, len(ints))
					for i, n := range ints {
						res[i] = n - 1
					}
					return res
				}
			}
		}

		res := search()
		list := make([]LocalCard, 0, len(res))
		for _, ix := range res {
			c, ok := db.CardAt(ix)
			if !ok {
				continue
			}
			if filter(c) {
				list = append(list, NewLocalCard(c, ix))
			}
		}

		return list, nil
	}

	handleInputLine := func(line string) {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)

		isCommand, err := handleCommand(fields)
		printErr(err)
		if isCommand {
			return
		}

		switch state.Mode {
		case ModeCollection:
			options, err := searchLocal(fields)
			if err != nil {
				printErr(err)
				return
			}
			if len(options) == 0 {
				printErr(errors.New("no results"))
				return
			}

			max := 20
			if line != "" {
				max = 0
			}
			// const max = 20
			// skip := 0
			// total := len(options)
			// if len(options) > max && line == "" {
			// 	skip = len(options) - max
			// 	//options = options[len(options)-max:]
			// }

			roptions := make([]mtgjson.Card, 0, len(options))
			for _, c := range options {
				rc, ok := cardByUUID(c.UUID())
				if ok {
					roptions = append(roptions, rc)
				}
			}

			modifyState(false, func(s State) State {
				s.Local = options
				s.Options = roptions
				return s
			})
			printOptions(max)

			return

		case ModeSearch:
			if line == "" {
				printOptions(0)
				return
			}
			options := searchAll(line)
			if len(options) == 0 {
				printErr(errors.New("no results"))
				return
			} else if len(options) <= 10000 {
				modifyState(false, func(s State) State {
					s.Options = options
					return s
				})
				printOptions(0)
				return
			}

			printErr(errors.New("too many results, try a more specific query"))
			return
		case ModeAdd:
			if line == "" {
				return
			}
			options := searchAll(line)
			if len(options) == 0 {
				printErr(errors.New("no results"))
				return
			} else if len(options) == 1 {
				addToCollection(options)
				return
			} else if len(options) < 100 {
				modifyState(true, func(s State) State {
					s.Options = options
					s.Mode = ModeSelect
					return s
				})
				printOptions(0)
				return
			}

			printErr(errors.New("too many results, try a more specific query"))
			return

		case ModeSelect:
			sel := make([]mtgjson.Card, 0, 1)
			for _, c := range state.Options {
				if strings.Contains(strings.ToLower(string(c.UUID)), strings.ToLower(line)) {
					sel = append(sel, c)
				}
			}

			if len(sel) > 1 {
				printOptions(0)
				if line != "" {
					printErr(errors.New("multiple matches, try more specific query"))
				}
				return
			} else if len(sel) == 0 {
				printOptions(0)
				printErr(errors.New("no card matches that uuid"))
				return
			}

			addToCollection(sel)
			modifyState(false, func(s State) State {
				s.Options = nil
				s.Mode = s.PrevMode
				return s
			})
		}
	}

	if !skipIntro {
		fmt.Println("Type /help for usage information")
		fmt.Println("Type /exit to quit or close stdin (Ctrl-d)")
		fmt.Println("press enter to continue...")
		func() {
			b := make([]byte, 1)
			for {
				_, err := io.ReadFull(os.Stdin, b)
				exit(err)
				if b[0] == 10 {
					break
				}
			}
		}()
	}

	for _, arg := range flag.Args() {
		print(fmt.Sprintf("> %s", arg))
		handleInputLine(arg)
	}

	scan := bufio.NewScanner(os.Stdin)
	scan.Split(bufio.ScanLines)

	inputCh := make(chan string, 1)
	go func() {
		for scan.Scan() {
			inputCh <- scan.Text()
		}
		exit(scan.Err())
		bye()
	}()

	prompt := func() {
		switch state.Mode {
		case ModeAdd:
			printDiv()
			print("Search all and add to collection")
			print("> ")
		case ModeCollection:
			printDiv()
			print("Search collection (card name or range)")
			print("> ")
		case ModeSearch:
			printDiv()
			print("Search all")
			print("> ")
		case ModeSelect:
			printDiv()
			print("Enter (partial) UUID to select a card")
			listID := cardListID(state.Options)
			if lastImageListID != listID {
				print("Run /images to view images")
			}
			print("UUID > ")
		default:
			printDiv()
			print("> ")
		}
		flush()
	}

	prompt()
	refresh()
	for {
		select {
		case <-cancelCh:
			modifyState(true, func(s State) State {
				switch s.Mode {
				case ModeSelect:
					s.Mode = s.PrevMode
					s.Options = nil
					s.Local = nil
				}
				return s
			})
			prompt()
		case <-refreshCh:
			handleInputLine("")
			prompt()
		case txt := <-inputCh:
			handleInputLine(txt)
			prompt()
		}
	}
}
