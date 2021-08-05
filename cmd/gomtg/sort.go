package main

import "sort"

type Sortable struct {
	swap func(i, j int)
	len  int

	dataInt []int
	dataStr []string
}

func NewSortable(swap func(i, j int)) *Sortable {
	return &Sortable{swap: swap}
}

func (s *Sortable) SetData(ints []int, strings []string) {
	if len(ints) != 0 && len(strings) != 0 {
		panic("sorting data should either be strings or ints")
	}
	s.dataInt = ints
	s.dataStr = strings
	s.len = len(ints)
	if s.len == 0 {
		s.len = len(strings)
	}
}

func (s *Sortable) Sort() { sort.Stable(s) }

func (s *Sortable) Len() int { return s.len }
func (s *Sortable) Less(i, j int) bool {
	if len(s.dataInt) != 0 {
		return s.dataInt[i] < s.dataInt[j]
	}
	if len(s.dataStr) != 0 {
		return s.dataStr[i] < s.dataStr[j]
	}
	return false
}
func (s *Sortable) Swap(i, j int) {
	s.swap(i, j)
	if len(s.dataInt) != 0 {
		s.dataInt[i], s.dataInt[j] = s.dataInt[j], s.dataInt[i]
	}
	if len(s.dataStr) != 0 {
		s.dataStr[i], s.dataStr[j] = s.dataStr[j], s.dataStr[i]
	}
}
