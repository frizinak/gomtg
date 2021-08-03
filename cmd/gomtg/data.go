package main

import (
	"fmt"
	"os"

	"github.com/frizinak/gomtg/mtgjson"
)

func loadData(file string) (mtgjson.AllPrintings, error) {
	if _, err := os.Stat(file); err != nil {
		destJSON := file + ".json"
		err := progress("Download mgtjson.com data", func() error {
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

		err = progress("Encode to gob", func() error {
			in, err := os.Open(destJSON)
			if err != nil {
				return err
			}
			defer in.Close()

			tmp := file + ".tmp"
			out, err := os.Create(tmp)
			if err != nil {
				return err
			}

			data, err := mtgjson.ReadAllPrintingsJSON(in)
			if err != nil {
				out.Close()
				return err
			}

			if err := mtgjson.WriteAllPrintingsGOB(out, data); err != nil {
				out.Close()
				return err
			}

			out.Close()
			if err := os.Rename(tmp, file); err != nil {
				return err
			}
			return os.Remove(destJSON)
		})
		if err != nil {
			return nil, err
		}
	}

	var data mtgjson.AllPrintings
	err := progress("Parse mtgjson.com data", func() error {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		data, err = mtgjson.ReadAllPrintingsGOB(f)
		if err != nil {
			err = fmt.Errorf(
				"%w: data seems invalid, delete '%s'. It will be redownloaded next run.",
				err,
				file,
			)
		}
		return err
	})

	return data, err
}
