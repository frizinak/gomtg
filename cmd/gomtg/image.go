package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"

	"github.com/frizinak/gomtg/mtgjson"
)

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

func getImage(url string) (image.Image, error) {
	r, w := io.Pipe()
	var werr error
	go func() {
		if err := downloadImage(url, w); err != nil {
			werr = err
			w.Close()
		}
	}()
	img, _, err := image.Decode(r)
	if err != nil {
		return img, err
	}
	return img, werr
}

func downloadImage(url string, w ...io.Writer) error {
	dl := time.Now().Add(time.Second * 10)
	ctx, cancel := context.WithDeadline(context.Background(), dl)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading image failed with status code: %s", res.Status)
	}
	_, err = io.Copy(io.MultiWriter(w...), res.Body)
	return err
}

func getImageCached(url string, dir string) (image.Image, error) {
	s := sha256.Sum256([]byte(url))
	h := hex.EncodeToString(s[:])
	path := filepath.Join(dir, h+".jpg")
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		img, _, err := image.Decode(f)
		return img, err
	}
	f.Close()

	if os.IsNotExist(err) {
		tmp := tmpFile(path)
		f, err = os.Create(tmp)
		if err != nil {
			return nil, err
		}
		pr, pw := io.Pipe()
		var gerr error
		done := make(chan struct{}, 1)
		go func() {
			err := downloadImage(url, f, pw)
			f.Close()
			pw.Close()
			if err != nil {
				gerr = err
				os.Remove(tmp)
				return
			}
			gerr = os.Rename(tmp, path)
			if gerr != nil {
				os.Remove(tmp)
			}
			done <- struct{}{}
		}()
		img, _, err := image.Decode(pr)
		if err != nil {
			return nil, err
		}
		<-done
		return img, gerr
	}

	return nil, err
}

func addUUIDsToCollage(cols, rows int, canvas *image.NRGBA, cards []mtgjson.Card, imgs []image.Rectangle) error {
	fontLSrc := image.NewUniform(color.NRGBA{30, 0, 0, 180})
	fontHSrc := image.NewUniform(color.NRGBA{200, 0, 0, 255})
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

	narrowest := -1
	for _, i := range imgs {
		w := i.Bounds().Dx()
		if narrowest < 0 || w < narrowest {
			narrowest = w
		}
	}

	b := canvas.Bounds()
	canvasW, canvasH := b.Dx(), b.Dy()

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

	strs := make([]string, len(cards))
	for i, c := range cards {
		strs[i] = string(c.UUID)
	}

	uniq := uniqUUIDPart(strs)
	dwr := font.Drawer{
		Dst:  fontLayer,
		Src:  fontLSrc,
		Face: face,
	}
	for ix, dst := range imgs {
		str := string(cards[ix].UUID)
		dwr.Dot = fixed.P(0, 0)
		txtBounds, _ := dwr.BoundString(str)
		width := float64((txtBounds.Max.X - txtBounds.Min.X).Round())

		pt := image.Pt(
			int(float64(dst.Min.X+dst.Dx()/2)/fontScale-width/2),
			int(float64(dst.Min.Y+20)/fontScale),
		)
		dwr.Dot = fixed.P(pt.X, pt.Y)

		box := image.Rect(
			txtBounds.Min.X.Floor()-10,
			txtBounds.Min.Y.Floor()-4,
			txtBounds.Max.X.Ceil()+10,
			txtBounds.Max.Y.Ceil()+4,
		).Add(pt)
		draw.Draw(fontLayer, box, fontBGSrc, image.Point{}, draw.Over)

		dwr.DrawString(uniq[ix][0])
		if len(uniq) != 1 {
			dwr.Src = fontHSrc
		}
		dwr.DrawString(uniq[ix][1])
		dwr.Src = fontLSrc
		dwr.DrawString(uniq[ix][2])

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

	return nil
}

func genCollage(cols, rows int, imgs []image.Image) (*image.NRGBA, []image.Rectangle, error) {
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
	rects := make([]image.Rectangle, len(imgs))
	for y := 0; y < rows; y++ {
		maxHeight := 0
		for x := 0; x < cols; x++ {
			ix := y*cols + x
			if ix >= len(imgs) {
				break
			}
			dst := imgs[ix].Bounds()
			dst = dst.Add(offset)

			draw.Draw(canvas, dst, imgs[ix], image.Point{}, draw.Src)
			rects[ix] = dst

			offset.X += dst.Dx()
			h := dst.Dy()
			if h > maxHeight {
				maxHeight = h
			}
		}
		offset.X = 0
		offset.Y += maxHeight
	}

	return canvas, rects, nil
}

type ImageGetter func(url string) (image.Image, error)

func genImages(cards []mtgjson.Card, file string, getImage ImageGetter, progress func(n, total int)) error {
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
					continue
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

	canvas, rects, err := genCollage(cols, rows, imgs)
	if err != nil {
		return err
	}

	if err := addUUIDsToCollage(cols, rows, canvas, cards, rects); err != nil {
		return err
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}

	err = jpeg.Encode(f, canvas, &jpeg.Options{Quality: 80})
	_ = f.Close()
	if err != nil {
		return err
	}
	_ = f.Sync()
	return nil
}
