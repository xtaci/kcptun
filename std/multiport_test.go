// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package std

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
)

func TestDial(t *testing.T) {
	reg := regexp.MustCompile(`(.*)\:([0-9]{1,5})-?([0-9]{1,5})?`)
	matches := reg.FindStringSubmatch("www.unknown.unknown:20000-21000")
	for i := 0; i < len(matches); i++ {
		fmt.Println(matches[i])
	}

	minPort, err := strconv.Atoi(matches[2])
	if err != nil {
		t.Fatal(err)
	}
	maxPort, err := strconv.Atoi(matches[3])
	if err != nil {
		t.Fatal(err)
	}

	t.Log("minport:", minPort)
	t.Log("maxport:", maxPort)

	remoteAddr := fmt.Sprintf("%v:%v", matches[1], uint64(minPort)+1000%uint64(maxPort-minPort+1))

	t.Log("RemoteAddr:", remoteAddr)

	testcase2 := "1.2.3.4:20000"
	matches = reg.FindStringSubmatch(testcase2)
	for i := 0; i < len(matches); i++ {
		t.Log(testcase2, "submatch", i, matches[i])
	}

	testcase3 := ":20000-20001"
	matches = reg.FindStringSubmatch(testcase3)
	for i := 0; i < len(matches); i++ {
		t.Log(testcase3, "submatch", i, matches[i])
	}

	testcase4 := ":20000"
	matches = reg.FindStringSubmatch(testcase4)
	for i := 0; i < len(matches); i++ {
		t.Log(testcase4, "submatch", i, matches[i])
	}

}
