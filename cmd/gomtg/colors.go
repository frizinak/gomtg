package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Colors map[string][3]int

func (c Colors) Encode() string {
	kv := make([]string, 0, len(c))
	for i := range c {
		kv = append(kv, fmt.Sprintf("%s:%d:%d:%d", i, c[i][0], c[i][1], c[i][2]))
	}
	sort.Strings(kv)
	return strings.Join(kv, ",")
}

func (c Colors) Merge(nc Colors) Colors {
	for i := range nc {
		if _, ok := c[i]; !ok {
			c[i] = nc[i]
		}
	}

	return c
}

func (c Colors) Get(n string) string {
	clrs := c[n]
	s := make([]string, 0, 2)
	if clrs[0] != 0 {
		s = append(s, strconv.Itoa(clrs[0]+39))
	}
	if clrs[1] != 0 {
		s = append(s, strconv.Itoa(clrs[1]+29))
	}
	if clrs[2] == 1 {
		s = append(s, "1")
	}
	n = strings.Join(s, ";")
	if n == "" {
		return ""
	}
	return fmt.Sprintf("\033[%sm", n)
}

func DecodeColors(s string) (Colors, error) {
	c := make(Colors)

	items := strings.Split(s, ",")
	for _, item := range items {
		p := strings.SplitN(item, ":", 4)
		if len(p) < 2 {
			return c, fmt.Errorf("'%s' is not a valid key:value", item)
		}
		for i := range p {
			p[i] = strings.TrimSpace(p[i])
		}

		var list [3]int
		for i, v := range p[1:] {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c, fmt.Errorf("'%s' contains an invalid integer", item)
			}
			list[i] = n
		}
		c[p[0]] = list
	}

	return c, nil
}
