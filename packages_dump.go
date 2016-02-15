// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func dumpPackages(root string, nworkers int, doMD5, doSHA1 bool) {

	var (
		scanner = packageScanner{
			doMD5:  doMD5,
			doSHA1: doSHA1,
		}
		now = time.Now()
	)

	filepath.Walk(root, func(path string, fi os.FileInfo, walkerr error) error {
		if fi == nil {
			log.Printf("no such file or directory: %s\n", path)
			return nil
		}
		if !fi.IsDir() {
			return nil
		}

		log.Println("start building index from", path)

		if err := scanner.scan(path, nworkers); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
		return nil
	})
	scanner.packages.StringTo(os.Stdout)
	log.Printf("time to parse %d packages: %s\n",
		scanner.packages.Len(), time.Since(now))
	log.Println("done building index")

	return
}
