package main

import (
	"fmt"
	"regexp"
	"testing"
)

func TestDial(t *testing.T) {
	reg := regexp.MustCompile(`(.*)\:([0-9]{1,5})-?([0-9]{1,5})?`)
	strs := reg.FindStringSubmatch("0.0.0.0:20000-21000")
	for i := 0; i < len(strs); i++ {
		fmt.Println(strs[i])
	}

	strs = reg.FindStringSubmatch("0.0.0.0:20000")
	for i := 0; i < len(strs); i++ {
		fmt.Println(strs[i])
	}

}
