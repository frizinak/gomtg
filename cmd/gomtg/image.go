package main

import (
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
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

	narrowest := -1
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
			if narrowest < 0 || w < narrowest {
				narrowest = w
			}
		}
		canvasH += maxHeight
	}
	for _, w := range widths {
		canvasW += w
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, canvasW, canvasH))

	fontSrc := image.NewUniform(color.NRGBA{60, 0, 0, 255})
	fontBGSrc := image.NewUniform(color.NRGBA{204, 204, 204, 180})
	//face := basicfont.Face7x13
	fontScale := 1.0

	col, err := opentype.ParseCollection(gobold.TTF)
	if err != nil {
		return err
	}
	gofont, err := col.Font(0)
	if err != nil {
		return err
	}

	face, err := opentype.NewFace(gofont, &opentype.FaceOptions{
		Size:    math.Max(10, float64(narrowest)/36*1.5),
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return err
	}

	fontLayer := canvas
	if fontScale != 1 {
		fontLayer = image.NewNRGBA(image.Rect(
			0, 0,
			int(float64(canvasW)/fontScale), int(float64(canvasH)/fontScale),
		))
	}

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

			dwr := font.Drawer{
				Dst:  fontLayer,
				Src:  fontSrc,
				Face: face,
			}
			str := string(cards[ix].UUID)
			txtBounds, _ := dwr.BoundString(str)
			width := float64((txtBounds.Max.X - txtBounds.Min.X).Round())

			dwr.Dot = fixed.P(
				int(float64(offset.X+dst.Dx()/2)/fontScale-width/2),
				int(float64(offset.Y+20)/fontScale),
			)

			txtBounds, _ = dwr.BoundString(str)
			txtDst := image.Rect(
				txtBounds.Min.X.Floor()-10,
				txtBounds.Min.Y.Floor()-4,
				txtBounds.Max.X.Ceil()+10,
				txtBounds.Max.Y.Ceil()+4,
			)
			draw.Draw(fontLayer, txtDst, fontBGSrc, image.Point{}, draw.Over)
			dwr.DrawString(str)

			offset.X += dst.Dx()
			h := dst.Dy()
			if h > maxHeight {
				maxHeight = h
			}
		}
		offset.X = 0
		offset.Y += maxHeight
	}

	if fontScale != 1 {
		draw.NearestNeighbor.Transform(
			canvas,
			f64.Aff3{
				fontScale, 0, 0,
				0, fontScale, 0,
			},
			fontLayer,
			fontLayer.Bounds(),
			draw.Over,
			nil,
		)
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, canvas, &jpeg.Options{Quality: 80})
}
