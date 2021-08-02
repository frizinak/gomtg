package mtgjson

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"io"
	"net/http"
)

func init() {
	gob.Register(AllPrintings{})
}

func DownloadAllPrintings(w io.Writer) error {
	res, err := http.Get("https://mtgjson.com/api/v5/AllPrintings.json.gz")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	r, err := gzip.NewReader(res.Body)
	if err != nil {
		return err
	}

	if _, err = io.Copy(w, r); err != nil {
		return err
	}

	return r.Close()
}

func ReadAllPrintingsJSON(r io.Reader) (AllPrintings, error) {
	j := json.NewDecoder(r)
	d := allPrintings{}
	if err := j.Decode(&d); err != nil {
		return nil, err
	}
	return d.Data, nil
}

func ReadAllPrintingsGOB(r io.Reader) (AllPrintings, error) {
	dec := gob.NewDecoder(r)
	d := AllPrintings{}
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	return d, nil
}

func WriteAllPrintingsGOB(w io.Writer, p AllPrintings) error {
	enc := gob.NewEncoder(w)
	return enc.Encode(p)
}

type allPrintings struct {
	// Meta Meta             `json:"meta"`
	Data AllPrintings `json:"data"`
}
