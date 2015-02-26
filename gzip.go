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
	"os/exec"
	"time"
)

type Gzipper func(w io.Writer, r io.Reader) error

// use the compress/gzip to compress the content of
// 'r'.
func GzGolang(w io.Writer, r io.Reader) error {
	gz, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	gz.Header.ModTime = time.Now()
	if _, err := io.Copy(gz, r); err != nil {
		return err
	}
	gz.Close()
	return nil
}

// use a pipe to 'gzip' to create the .gz such that opkg
// accepts the output. right now it's unclear why opkg explodes
// when it hits a golang-native-created .gz file.
func GzGzipPipe(w io.Writer, r io.Reader) error {
	cmd := exec.Command("gzip", "-9", "-c")
	cmd.Stdin = r
	cmd.Stdout = w
	return cmd.Run()
}
