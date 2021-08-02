package fuzzy

import (
	"strings"
)

type Index struct {
	fuzzyLength int
	// items       []string
	n    int
	data map[string][]int
}

func NewIndex(fuzzyLength int, items []string) *Index {
	if fuzzyLength < 2 {
		fuzzyLength = 2
	}
	ix := &Index{
		fuzzyLength: fuzzyLength,
		n:           len(items),
		data:        make(map[string][]int, len(items)),
	}

	for i, v := range items {
		fuzzy := ix.parts(v)
		for _, p := range fuzzy {
			ix.data[p] = append(ix.data[p], i)
		}
	}

	return ix
}

type Include func(score, low, high int) bool

func (i *Index) Search(q string, include Include) []int {
	scores := make([]int, i.n)
	words := i.parts(q)
	min, max := -1, 0
	for _, q := range words {
		done := make(map[int]struct{})
		if b, ok := i.data[q]; ok {
			for i := 0; i < len(b); i++ {
				if _, ok := done[b[i]]; ok {
					continue
				}
				ix := b[i]
				scores[ix]++
				done[b[i]] = struct{}{}
			}
		}
	}

	for _, score := range scores {
		if score < min || min == -1 {
			min = score
		}
		if score > max {
			max = score
		}
	}

	ixes := make([]int, 0)
	for i, score := range scores {
		if include(score, min, max) {
			ixes = append(ixes, i)
		}
	}

	return ixes
}

func (index *Index) parts(q string) []string {
	qs := make([]string, 0, len(q))
	p := strings.Fields(
		strings.Trim(strings.TrimSpace(strings.ToLower(q)), "!@#$%^&*=./,"),
	)
	for i := range p {
		if len(p[i]) < 2 {
			continue
		}
		if len(p[i]) <= index.fuzzyLength {
			qs = append(qs, p[i])
			continue
		}
		for j := 0; j < len(p[i])-index.fuzzyLength+1; j++ {
			qs = append(qs, strings.TrimSpace(p[i][j:j+index.fuzzyLength]))
		}
	}

	return qs
}
