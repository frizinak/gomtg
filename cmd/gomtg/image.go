package main

import (
	"errors"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"

	"github.com/frizinak/gomtg/mtgjson"
)

func getImage(url string) (image.Image, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	img, _, err := image.Decode(res.Body)
	return img, err
}

func genImages(cards []mtgjson.Card, file string, progress func(n, total int)) error {
	if len(cards) == 0 {
		return errors.New("no cards to fetch images for")
	}
	grid := float64(len(cards))
	_cols := math.Ceil(math.Sqrt(grid))
	_rows := math.Ceil(grid / _cols)
	cols, rows := int(_cols), int(_rows)

	urls := make([]string, 0, len(cards))
	for _, c := range cards {
		u, err := c.ImageURL()
		if err != nil {
			return err
		}
		urls = append(urls, u)
	}

	type result struct {
		ix int
		image.Image
	}
	type job struct {
		ix  int
		url string
	}

	const workers = 4
	work := make(chan job, workers)
	results := make(chan result, workers)
	var wg sync.WaitGroup
	var gerr error
	var total = len(cards)
	var dled uint32
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for j := range work {
				if gerr != nil {
					break
				}
				img, err := getImage(j.url)
				if err != nil {
					gerr = err
				}
				atomic.AddUint32(&dled, 1)
				progress(int(dled), int(total))
				results <- result{ix: j.ix, Image: img}
			}
			wg.Done()
		}()
	}

	imgs := make([]image.Image, len(cards))
	done := make(chan struct{})
	go func() {
		for r := range results {
			imgs[r.ix] = r
		}
		done <- struct{}{}
	}()

	for i, u := range urls {
		work <- job{ix: i, url: u}
	}

	close(work)
	wg.Wait()
	close(results)
	<-done
	if gerr != nil {
		return gerr
	}

	canvasH, canvasW := 0, 0
	widths := make([]int, cols)
	for y := 0; y < rows; y++ {
		maxHeight := 0
		for x := 0; x < cols; x++ {
			ix := y*cols + x
			if ix >= len(imgs) {
				break
			}
			b := imgs[ix].Bounds()
			w, h := b.Dx(), b.Dy()
			if h > maxHeight {
				maxHeight = h
			}
			if widths[x] < w {
				widths[x] = w
			}
		}
		canvasH += maxHeight
	}
	for _, w := range widths {
		canvasW += w
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, canvasW, canvasH))
	offset := image.Point{}
	for y := 0; y < rows; y++ {
		maxHeight := 0
		for x := 0; x < cols; x++ {
			ix := y*cols + x
			if ix >= len(imgs) {
				break
			}
			dst := imgs[ix].Bounds()
			draw.Draw(canvas, dst.Add(offset), imgs[ix], image.Point{}, draw.Src)

			offset.X += dst.Dx()
			h := dst.Dy()
			if h > maxHeight {
				maxHeight = h
			}
		}
		offset.X = 0
		offset.Y += maxHeight
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, canvas, &jpeg.Options{Quality: 80})
}
