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
	"syscall"
	"time"

	"github.com/frizinak/gomtg/fuzzy"
	"github.com/frizinak/gomtg/mtgjson"
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

func cardsString(db *DB, cards []mtgjson.Card, colors Colors, uniq bool) []string {
	l := make([]string, len(cards))
	uuids := make([]string, len(cards))
	for i, c := range cards {
		uuids[i] = string(c.UUID)
	}

	if uniq {
		uuids = colorUniqUUID(uuids, colors)
	}

	for i, c := range cards {
		l[i] = fmt.Sprintf(
			"%s \u2502 %-5s \u2502 %-4d \u2502 %s",
			uuids[i],
			c.SetCode,
			db.Count(c.UUID),
			c.Name,
		)
	}
	return l
}

func localCardsString(db *DB, cards []LocalCard, colors Colors, uniq bool) []string {
	l := make([]string, len(cards))
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

	for i, c := range cards {
		tags := c.Tags()
		tagstr := strings.Join(tags, ",")

		l[i] = fmt.Sprintf(
			"%6d \u2502 %s \u2502 %-5s \u2502 %-4d \u2502 %-"+titlePad+"s \u2502 %s",
			c.Index+1,
			uuids[i],
			c.SetID(),
			db.Count(c.UUID()),
			c.Name(),
			tagstr,
		)
	}
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
	flag.Parse()

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
		locker.Unlock()
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

	state := State{Mode: ModeCollection}
	output := make([]string, 1, 30)

	print := func(msg ...string) {
		output = append(output, msg...)
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
		clr := colors.Get("status")
		output[0] = fmt.Sprintf("%s %s \033[0m\n", clr, state.StringShort())
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
	printOptions := func() {
		if state.Mode == ModeCollection {
			print(localCardsString(db, state.Local, colors, true)...)
			return
		}
		print(cardsString(db, state.Options, colors, true)...)
	}

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

	_commandQ := func([]string) error {
		if len(queue) == 1 {
			print("Queue is empty")
			return nil
		}
		for _, s := range queue[1:] {
			print("- " + s.String(db, colors))
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
			print("/repeat | /r                  add last card again")
			print("/sets  <filter>               print all known sets (optionally filtered)")
			print("/undo   | /u                  remove last item from queue")
			print("/images | /imgs               create a collage of all cards in current list")
			print("/image  | /img <uuid>         show card image for card with (partial) UUID <uuid>")
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
			print("                                - search:        search all cards (fuzzy)")
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

			tags := state.Tagging
			state.Tagging = nil
			for i := range queue {
				queue[i].Tagging = nil
			}
			for _, t := range tags {
				t.Commit()
			}

			err := db.Save(dbFile)
			if err != nil {
				return err
			}

			rebuildLocalFuzz()
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
				printOptions()
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
			printOptions()
			printAlert(fmt.Sprintf("Downloaded image to '%s'", imagePath))
			return spawnViewer(imageCommand, imageRefreshCommand, imageAutoReload, imagePath)
		},
		"image": func(a []string) error {
			if len(a) != 1 || len(a[0]) == 0 {
				return errors.New("/img requires exactly 1 argument")
			}

			list := make([]mtgjson.Card, 0, 1)
			arg := a[0]
			add := func(c mtgjson.Card) error {
				if !strings.Contains(
					strings.ToLower(string(c.UUID)),
					strings.ToLower(arg),
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

			for _, c := range state.Options {
				if err := add(c); err != nil {
					return err
				}
			}

			if len(list) != 1 {
				for _, c := range cards {
					if err := add(c); err != nil {
						return err
					}
				}
			}

			if len(list) == 0 {
				return errors.New("no such card")
			}

			listID := cardListID(list)
			if lastImageListID != listID {
				err := genImages(list, imagePath, imageGetter, func(n, total int) {})
				if err != nil {
					return err
				}
				lastImageListID = listID
			}

			printOptions()
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
		"repeat": func([]string) error {
			if len(state.Selection) == 0 {
				return errors.New("nothing to repeat")
			}
			addToCollection(state.Selection[len(state.Selection)-1:].Cards())

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

	handleCommand := func(f []string) (bool, error) {
		isCommand := false
		if len(f) == 0 {
			return false, nil
		}

		if strings.HasPrefix(f[0], "/") {
			d := f[0][1:]
			if cmd, ok := commands[d]; ok {
				return true, cmd(f[1:])
			}
			return true, fmt.Errorf("no such command: '%s'", f[0])
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
	searchLocal := func(qry string) ([]LocalCard, error) {
		filter := func(c *Card) bool {
			return state.FilterSet == "" || c.SetID() == state.FilterSet
		}
		if qry == "" {
			all := db.Cards()
			list := make([]LocalCard, 0, len(all))
			for i, c := range all {
				if filter(c) {
					list = append(list, NewLocalCard(c, i))
				}
			}
			return list, nil
		}

		search := func() []int {
			return localFuzz.Search(qry, func(score, min, max int) bool {
				return score > 0 && score == max
			})
		}

		if numericRE.MatchString(qry) {
			ints, ok := intRange(qry)
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
				return list, errors.New(`Invalid item/range`)
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
			options, err := searchLocal(line)
			if err != nil {
				printErr(err)
				return
			}
			if len(options) == 0 {
				printErr(errors.New("no results"))
				return
			}

			const max = 20
			skip := len(options) - max
			if len(options) > max && line == "" {
				printAlert(fmt.Sprintf("Skipping %d cards, last %d:", skip, max))
				options = options[len(options)-max:]
			}

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
			printOptions()
			return

		case ModeSearch:
			if line == "" {
				printOptions()
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
				printOptions()
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
				printOptions()
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
				printOptions()
				if line != "" {
					printErr(errors.New("multiple matches, try more specific query"))
				}
				return
			} else if len(sel) == 0 {
				printOptions()
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
			print("________________________________")
			print("Search all and add to collection")
			print("> ")
		case ModeCollection:
			print("_________________")
			print("Search collection (card name or range)")
			print("> ")
		case ModeSearch:
			print("__________")
			print("Search all")
			print("> ")
		case ModeSelect:
			print("_____________________________________")
			print("Enter (partial) UUID to select a card")
			listID := cardListID(state.Options)
			if lastImageListID != listID {
				print("Run /images to view images")
			}
			print("UUID > ")
		default:
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
