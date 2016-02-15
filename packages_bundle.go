// This file is part of *kellner*
//
// Copyright (C) 2016, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// write a bundle of all packages in the index
// a bundle can be a zip-archive/tar-archive/text-file
// (currently implemented: zip-archive and primitive
// list to stdout)
func (self *packageIndex) writeBundle(output string, archsubdirs bool) error {
	now := time.Now()
	// now let us create a zipfile with the condensated files
	if output == "-" {
		// this is more a debugging thing,
		// output filenames to stdout
		fmt.Printf("sorted names:\n%s\n", strings.Join(self.SortedNames(), "\n"))
	} else if strings.HasSuffix(output, ".zip") {
		zipfile, err := os.Create(output)
		if err != nil {
			return err
		}
		defer zipfile.Close()

		zipper := zip.NewWriter(zipfile)
		defer zipper.Close()

		for _, pkg := range self.Entries {
			header := &zip.FileHeader{Name: pkg.Name}
			if archsubdirs {
				header.Name = filepath.Join(pkg.Header["Architecture"], header.Name)
			}
			header.SetModTime(now)
			writer, zwerr := zipper.CreateHeader(header)
			if zwerr != nil {
				return zwerr
			}
			// read scanned package from original location
			sourcepkg, srcerr := os.Open(pkg.ScanLocation)
			if srcerr != nil {
				return srcerr
			}
			// and write bytes into the zip/bundle
			_, cperr := io.Copy(writer, sourcepkg)
			sourcepkg.Close()
			if cperr != nil {
				return cperr
			}
		}
	}
	log.Printf("wrote %d into archive: %s. done after %s\n",
		self.Len(), output, time.Since(now))
	return nil
}
