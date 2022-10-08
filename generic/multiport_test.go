package generic

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
