package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func exportCSV(cards []LocalCard, dir string) (string, error) {
	file := filepath.Join(
		dir,
		fmt.Sprintf("export-%s.csv", time.Now().Format("2006-02-01_15-04-05")),
	)
	f, err := os.Create(file)
	if err != nil {
		return file, err
	}
	w := csv.NewWriter(f)
	defer f.Close()

	recs := make([]string, 4)
	recs[0] = "Index"
	recs[1] = "Name"
	recs[2] = "Set Code"
	recs[3] = "Foil"
	if err := w.Write(recs); err != nil {
		return file, err
	}

	var foil string
	for _, c := range cards {
		foil = "0"
		if c.Foil() {
			foil = "1"
		}
		recs[0] = strconv.Itoa(c.Index)
		recs[1] = c.Name()
		recs[2] = string(c.SetID())
		recs[3] = foil

		if err := w.Write(recs); err != nil {
			return file, err
		}
	}

	w.Flush()
	return file, w.Error()
}
