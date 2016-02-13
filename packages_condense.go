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
	"strconv"
	"strings"
	"time"
	"unicode"
)

// recursively scan through a list of directories and find
// packages. per package (and architecture) keep only the
// most recent version.
// output the list (plan: create zip) of resultant packages
func condensePackages(roots []string, nworkers int, output string, archsubdirs bool) error {
	var (
		scanner = packageScanner{
			doMD5:  false,
			doSHA1: false,
		}
		now = time.Now()
	)
	for _, root := range roots {
		werr := filepath.Walk(root, func(path string, fi os.FileInfo, walkerr error) error {
			if fi == nil {
				log.Printf("no such file or directory: %s\n", path)
				return nil
			}
			if !fi.IsDir() {
				return nil
			}
			log.Println("condensing packages from", path)

			if err := scanner.scan(path, nworkers); err != nil {
				return fmt.Errorf("error: %v\n", err)
			}
			return nil
		})
		if werr != nil {
			return werr
		}
	}
	if scanner.packages == nil {
		log.Printf("no packages found.\n")
		return fmt.Errorf("no packages found.")
	}

	// and now to the act of condensing...
	log.Printf("condensing a set of %d packages\n", scanner.packages.Len())
	condensate := &packageIndex{Entries: make(map[string]*ipkArchive)} // key is Package+Architecture
	for pkgname, pkg := range scanner.packages.Entries {

		// slight paranoia check (control data ./. pkgname):
		epochless_version := pkg.Header["Version"]
		if idx := strings.Index(pkg.Header["Version"], ":"); idx > 0 {
			epochless_version = pkg.Header["Version"][idx+1:]
		}
		synthetic_pkgname := fmt.Sprintf("%s_%s_%s.ipk",
			pkg.Header["Package"], epochless_version, pkg.Header["Architecture"])
		if synthetic_pkgname != pkgname {
			return fmt.Errorf("package %s has mismatching control-information \"%s\"",
				pkgname, synthetic_pkgname)
		}
		cname := pkg.Header["Architecture"] + pkg.Header["Package"] // condense-key
		if prev, ok := condensate.Entries[cname]; ok {
			// compare version
			cmp := compareVersion(pkg.Header["Version"], prev.Header["Version"])
			if cmp > 0 { // new finding wins
				fmt.Printf("%s: %s replaced: %s\n", pkg.Header["Package"],
					pkg.Header["Version"], prev.Header["Version"])
				condensate.Add(cname, pkg)
			}
		} else {
			// first finding
			condensate.Add(cname, pkg)
		}
	}

	// now let us create a zipfile with the condensated files
	if output == "-" {
		// this is more a debugging thing,
		// output filenames to stdout
		fmt.Printf("sorted names:\n%s\n", strings.Join(condensate.SortedNames(), "\n"))
	} else if strings.HasSuffix(output, ".zip") {
		zipfile, err := os.Create(output)
		if err != nil {
			return err
		}
		defer zipfile.Close()

		zipper := zip.NewWriter(zipfile)
		defer zipper.Close()

		for _, pkg := range condensate.Entries {
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
	log.Printf("condensed %d into %d packages. done after %s\n",
		scanner.packages.Len(), condensate.Len(), time.Since(now))
	return nil
}

// compare to versions,
// return 0 when equal or 1/-1 when different
func compareVersion(v1 string, v2 string) int {
	v1ord := versionStringToOrdinals(v1)
	v2ord := versionStringToOrdinals(v2)

	// make them equal in length, append zeros to the shorter one:
	lendiff := len(v1ord) - len(v2ord)
	if lendiff < 0 {
		v1ord = append(v1ord, make([]int, (-1*lendiff))...)
	} else if lendiff > 0 {
		v2ord = append(v2ord, make([]int, (lendiff))...)
	}

	for i := range v1ord {
		if v1ord[i] < v2ord[i] {
			return -1
		}
		if v1ord[i] > v2ord[i] {
			return 1
		}
	}
	return 0
}

// convert a string into an array of numbers parsed from
// all occurances of consecutive digits
// "foo123bar321" --> [123, 321]
// "version-1.0.7-rc3.3" --> [1,0,7,3,3]
func versionStringToOrdinals(version_string string) []int {
	ord := ""
	ordarray := []int{}
	addComponent := func() {
		if len(ord) > 0 {
			i, _ := strconv.Atoi(ord) // error is unlikely with all digits
			ordarray = append(ordarray, i)
			ord = ""
		}
	}
	for _, c := range version_string {
		if !unicode.IsDigit(c) {
			addComponent()
			continue
		}
		ord += string(c)
	}
	addComponent()
	return ordarray
}
