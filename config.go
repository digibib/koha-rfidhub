package main

import (
	"encoding/json"
	"io/ioutil"
)

type config struct {
	TCPPort   string
	HTTPPort  string
	LogLevels string
}

func (c *config) fromFile(file string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, c)
	if err != nil {
		return err
	}
	return nil
}
