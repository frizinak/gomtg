package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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

func cardString(c mtgjson.Card) string {
	return fmt.Sprintf("%s \u2502 %-5s \u2502 %s", c.UUID, c.SetCode, c.Name)
}

func localCardString(c LocalCard) string {
	return fmt.Sprintf("%6d \u2502 %s \u2502 %-5s \u2502 %s", c.Index+1, c.UUID, c.SetID, c.Name)
}

func cardListID(c []mtgjson.Card) string {
	ids := make([]string, len(c))
	for i, card := range c {
		ids[i] = string(card.UUID)
	}

	return strings.Join(ids, " ")
}

func main() {
	var skipIntro bool
	var imageCommand string
	var imageAutoReload bool
	var dbFile string
	flag.BoolVar(&skipIntro, "n", false, "Skip intro")
	flag.StringVar(
		&imageCommand,
		"i",
		"",
		`Command to run to view images. {} will be replaced by the filename.
if no {} argument is found, it will be appended to the command.
e.g.: -i "chromium 'file://{}'",
e.g.: -i "imv" -ia`,
	)
	flag.BoolVar(
		&imageAutoReload,
		"ia",
		false,
		`Your image viewer wont be killed and respawned each time you view an image.
Only useful if your viewer auto reloads updated images (imv and feh for example)`,
	)
	flag.StringVar(&dbFile, "db", "gomtg.db", "Database file to use")
	flag.Parse()

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

	cacheDir, err := os.UserCacheDir()
	exit(err)

	dir := filepath.Join(cacheDir, "gomtg")
	dest := filepath.Join(dir, "v5-all-printings.gob")
	imagePath := filepath.Join(dir, "options.jpg")

	var db *DB
	var localFuzz *fuzzy.Index
	rebuildLocalFuzz := func() {
		list := make([]string, 0)
		for _, card := range db.Cards() {
			list = append(list, card.Name)
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
	exit(progress("Filter paper cards", func() error {
		data = data.FilterOnlineOnly(false)
		cards = data.Cards()
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

	state := State{Mode: ModeCol}
	output := make([]string, 1, 30)

	print := func(msg string) {
		output = append(output, msg)
	}
	printAlert := func(msg string) {
		print(fmt.Sprintf("\033[1;32m %s \033[0m", msg))
	}
	printErr := func(err error) {
		if err == nil {
			return
		}
		print(fmt.Sprintf("\033[31m%s\033[0m", err.Error()))
	}
	flush := func() {
		fmt.Print("\033[2J\033[0;0H")
		output[0] = fmt.Sprintf("\033[30;42m %s \033[0m\n", state.StringShort())
		fmt.Print(strings.Join(output, "\n"))
		output = make([]string, 1, 30)
	}

	queue := []State{state}

	modifyState := func(undoable bool, cb func(s State) State) {
		ostate := state
		state = cb(state)
		if undoable && !ostate.Equal(state) {
			queue = append(queue, state)
		}
	}

	lastImageListID := ""
	printOptions := func() {
		if state.Mode == ModeCol {
			for _, c := range state.Local {
				print(localCardString(c))
			}
			return
		}
		for _, c := range state.Options {
			print(cardString(c))
		}
	}

	printSets := func() {
		list := make([]string, 0, len(data))
		for _, d := range data {
			list = append(list, fmt.Sprintf("%s: %s", d.Code, d.Name))
		}
		sort.Strings(list)
		for _, item := range list {
			print(item)
		}
	}

	_commandQ := func([]string) error {
		if len(queue) == 1 {
			print("Queue is empty")
			return nil
		}
		for _, s := range queue[1:] {
			print("- " + s.String())
		}
		return nil
	}

	commands := map[string]func(arg []string) error{
		"help": func([]string) error {
			print("Usage:")
			print("SIGINT (Ctrl-c)       cancel action in progress")
			print("/help                 this")
			print("/exit  | /quit        quit")
			print("/queue | /q           view operation queue")
			print("/sets                 print all known sets")
			print("/undo  | /u           remove last item from queue")
			print("/images | /imgs       create a collage of all cards in current list")
			print("/image | /img <uuid>  show card image for card with (partial) UUID <uuid>")
			print("/commit               commit selection to file (empties selection)")
			print("/mode  | /m <mode>    enter <mode>")
			print("                        - add:    add cards by entering their name (fuzzy)")
			print("                        - col:    search your collection for cards (fuzzy)")
			print("                        - search: search all cards (fuzzy)")
			print("/set   | /s <set>     only operate on cards within the given set")
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
				db.Add(FromCard(c))
			}
			err := db.Save(dbFile)
			if err != nil {
				return err
			}

			rebuildLocalFuzz()
			return nil
		},
		"sets": func([]string) error {
			printSets()
			return nil
		},
		"images": func([]string) error {
			listID := cardListID(state.Options)
			if lastImageListID == listID {
				printOptions()
				return nil
			}
			err := genImages(state.Options, imagePath, func(i, total int) {
				print(fmt.Sprintf("Downloaded %02d/%02d", i, total))
				flush()
			})
			if err != nil {
				return err
			}
			lastImageListID = listID
			printOptions()
			printAlert(fmt.Sprintf("Downloaded image to '%s'", imagePath))
			return spawnViewer(imageCommand, imageAutoReload, imagePath)
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

				if len(list) > 0 {
					return errors.New("multiple cards match")
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
				err := genImages(list, imagePath, func(n, total int) {})
				if err != nil {
					return err
				}
				lastImageListID = listID
			}

			printOptions()
			printAlert(fmt.Sprintf("Downloaded image to '%s'", imagePath))
			return spawnViewer(imageCommand, imageAutoReload, imagePath)
		},
		"mode": func(args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			m := Mode(arg)
			c := 0
			if !m.ValidInput() {
				for _, mode := range AllInputModes {
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
				printSets()
				return fmt.Errorf("invalid set id: '%s'", arg)
			}
			modifyState(true, func(s State) State {
				s.FilterSet = fs
				return s
			})
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

	searchLocal := func(qry string) ([]LocalCard, error) {
		filter := func(c *Card) bool {
			return state.FilterSet == "" || c.SetID == state.FilterSet
		}
		if qry == "" {
			all := db.Cards()
			list := make([]LocalCard, 0, len(all))
			for i, c := range all {
				if filter(c) {
					list = append(list, LocalCard{c, i})
				}
			}
			return list, nil
		}

		res := localFuzz.Search(qry, func(score, min, max int) bool {
			return score > 0 && score == max
		})

		list := make([]LocalCard, 0, len(res))
		for _, ix := range res {
			c, ok := db.CardAt(ix)
			if !ok {
				// ugh?
				return list, errors.New(`An unexpected error occurred,
nothing bad though.
Fuzzy db seems out of sync with your collection db try committing a restarting.`)
			}
			if filter(c) {
				list = append(list, LocalCard{c, ix})
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
		case ModeCol:
			options, err := searchLocal(line)
			if err != nil {
				printErr(err)
				return
			}
			if len(options) == 0 {
				printErr(errors.New("no results"))
				return
			}

			roptions := make([]mtgjson.Card, 0, len(options))
			for _, c := range options {
				rc, err := c.Card.Card(cards)
				if err == nil {
					roptions = append(roptions, rc)
				}
			}

			modifyState(false, func(s State) State {
				s.Query = line
				s.Local = options
				s.Options = roptions
				return s
			})
			printOptions()
			return

			printErr(errors.New("too many results, try a more specific query"))
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
			} else if len(options) < 100 {
				modifyState(false, func(s State) State {
					s.Query = line
					s.Options = options
					s.PrevMode = s.Mode
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
				modifyState(true, func(s State) State {
					s.Query = line
					s.Selection = append(s.Selection, options...)
					return s
				})

				printAlert(fmt.Sprintf("Added %s", cardString(options[0])))
				return
			} else if len(options) < 100 {
				modifyState(true, func(s State) State {
					s.Query = line
					s.Options = options
					s.PrevMode = s.Mode
					s.Mode = ModeSel
					return s
				})
				printOptions()
				return
			}

			printErr(errors.New("too many results, try a more specific query"))
			return

		case ModeSel:
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

			modifyState(true, func(s State) State {
				s.Options = nil
				s.Mode = s.PrevMode
				s.Selection = append(s.Selection, sel[0])
				printAlert(fmt.Sprintf("Added %s", cardString(sel[0])))
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
		fmt.Println(arg)
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
		case ModeCol:
			print("_________________")
			print("Search collection")
			print("> ")
		case ModeSearch:
			print("__________")
			print("Search all")
			print("> ")
		case ModeSel:
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
	for {
		select {
		case <-cancelCh:
			modifyState(true, func(s State) State {
				switch s.Mode {
				case ModeSel:
					s.Query = ""
					s.Mode = s.PrevMode
					s.Options = nil
				}
				return s
			})
			prompt()
		case txt := <-inputCh:
			handleInputLine(txt)
			prompt()
		}
	}
}
