// The MIT License (MIT)
//
// Copyright (c) 2019 xtaci
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

//go:build linux

package tcpraw

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func init() {
	go sigHandler()
}

func sigHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh

	// gracefully shutdown all connection
	connListMu.Lock()
	var wg sync.WaitGroup
	wg.Add(connList.Len())
	for elem := connList.Front(); elem != nil; elem = elem.Next() {
		go func(conn *tcpConn) {
			conn.Close()
			wg.Done()
		}(elem.Value.(*tcpConn))
	}
	connListMu.Unlock()
	wg.Wait()
	os.Exit(0)
}
