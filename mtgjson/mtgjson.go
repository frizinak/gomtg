package mtgjson

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func init() {
	gob.Register(AllPrintings{})
}

type allPrintings struct {
	// Meta Meta             `json:"meta"`
	Data AllPrintings `json:"data"`
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

func WriteCardsGOB(dir string, cards []Card) error {
	w := 8
	ch := make(chan Card, w)
	errs := make(chan error, w)
	for i := 0; i < w; i++ {
		go func() {
			for c := range ch {
				err := WriteCardGOB(dir, c)
				errs <- err
			}
		}()
	}

	go func() {
		for _, c := range cards {
			ch <- c
		}
	}()

	var gerr error
	for range cards {
		err := <-errs
		if err != nil {
			gerr = err
		}
	}

	return gerr
}

type noopWriteCloser struct{ w io.Writer }
type noopReadCloser struct{ r io.Reader }

func (n *noopWriteCloser) Write(b []byte) (int, error) { return n.w.Write(b) }
func (n *noopWriteCloser) Close() error                { return nil }
func (n *noopReadCloser) Read(b []byte) (int, error)   { return n.r.Read(b) }
func (n *noopReadCloser) Close() error                 { return nil }

const compress = false

func compressWriter(w io.Writer) io.WriteCloser {
	if compress {
		return gzip.NewWriter(w)
	}
	return &noopWriteCloser{w}
}

func compressReader(r io.Reader) (io.ReadCloser, error) {
	if compress {
		return gzip.NewReader(r)
	}
	return &noopReadCloser{r}, nil
}

func WriteCardGOB(dir string, card Card) error {
	dir, fp, err := uuidFP(dir, string(card.UUID))
	if err != nil {
		return err
	}
	_ = os.MkdirAll(dir, 0700)
	tmp := tmpFile(fp)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	gz := compressWriter(f)
	enc := gob.NewEncoder(gz)
	encErr := enc.Encode(card)
	if err := gz.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	if encErr != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, fp)
}

func ReadCardsGOB(dir string, cb func(Card)) error {
	g, err := filepath.Glob(filepath.Join(dir, "*", "*", "*"))
	if err != nil {
		return err
	}
	for _, fp := range g {
		uuid := filepath.Base(fp)
		c, err := ReadCardGOB(dir, UUID(uuid))
		if err != nil {
			return err
		}
		cb(c)
	}
	return nil
}

func ReadCardGOB(dir string, uuid UUID) (Card, error) {
	_, fp, err := uuidFP(dir, string(uuid))
	var fc Card
	if err != nil {
		return fc, err
	}
	f, err := os.Open(fp)
	if err != nil {
		return fc, err
	}
	gz, err := compressReader(f)
	if err != nil {
		f.Close()
		return fc, err
	}
	dec := gob.NewDecoder(gz)
	err = dec.Decode(&fc)
	f.Close()
	return fc, err
}

func uuidFP(dir string, uuid string) (string, string, error) {
	if len(uuid) != 36 {
		return "", "", errors.New("invalid uuid")
	}
	d1, d2 := uuid[0:2], uuid[2:4]
	rdir := filepath.Join(dir, d1, d2)
	return rdir, filepath.Join(rdir, uuid), nil
}

func tmpFile(file string) string {
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	rnd := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, rnd)
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf(
		"%s.%s-%s.tmp",
		file,
		stamp,
		base64.RawURLEncoding.EncodeToString(rnd),
	)
}
