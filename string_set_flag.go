package main

import (
	"flag"
	"strings"
)

type StringSet map[string]bool

func (s StringSet) String() string {
	var keys []string
	for key, _ := range s {
		keys = append(keys, key)
	}
	return strings.Join(keys, ",")
}

func (s *StringSet) Set(value string) error {
	(*s)[value] = true
	return nil
}

var _ flag.Value = &StringSet{}
