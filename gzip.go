// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"os/exec"
	"sync"
	"time"
)

type gzWrite func(w io.Writer, r io.Reader) error

// gzip.NewWriter() allocates a big chunk of memory and gives
// the garbage collector a hard time to pick it up correctly.
// gzGolangPool helps reusing such a "memory heavy" resource.
var gzGolangPool = sync.Pool{
	New: func() interface{} {
		var gz, _ = gzip.NewWriterLevel(ioutil.Discard, gzip.BestCompression)
		return gz
	},
}

// gzGolang uses compress/gzip to compress the content of
// 'r'.
func gzGolang(w io.Writer, r io.Reader) error {
	var gz = gzGolangPool.Get().(*gzip.Writer)
	defer gzGolangPool.Put(gz)
	gz.Reset(w)
	gz.Header.ModTime = time.Now()
	if _, err := io.Copy(gz, r); err != nil {
		return err
	}
	gz.Flush()
	return nil
}

// gzGzipPipe uses a pipe to 'gzip' (the executable) to create
// the .gz such that opkg accepts the output. right now it's
// unclear why opkg explodes when it hits a golang-native-created .gz
// file.
func gzGzipPipe(w io.Writer, r io.Reader) error {
	cmd := exec.Command("gzip", "-9", "-c")
	cmd.Stdin = r
	cmd.Stdout = w
	return cmd.Run()
}
