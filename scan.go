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
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

func scanRoot(root, cache string, nworkers int, doMD5, doSHA1 bool, gzipper gzWrite) {

	filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {

		if fi == nil || !fi.IsDir() {

			// not existing root-directory. we don't crash or exit here, the
			// operator might create it later on and trigger a rescan.
			if fi == nil && root == path {
				log.Printf("warning: not existent root directory %q\n", root)
			}

			return nil
		}

		// skip cache-directory
		if strings.HasPrefix(path, cache) {
			return filepath.SkipDir
		}

		var (
			now          = time.Now()
			relPath, _   = filepath.Rel(root, path)
			cachePath, _ = filepath.Abs(filepath.Join(cache, relPath))
			scanner      = packageScanner{
				cache:  cachePath,
				doMD5:  doMD5,
				doSHA1: doSHA1,
			}
		)

		if err = scanner.scan(path, nworkers); err != nil {
			log.Printf("error: %v", err)
			return nil
		}

		log.Printf("done building index for %q", path)
		log.Printf("time to parse %d packages in %q: %s\n",
			scanner.packages.Len(), path, time.Since(now))

		//
		// write the package index
		//

		var indexName = filepath.Join(cachePath, "Packages")
		var indexNameGz = indexName + ".gz"
		var indexNameStamps = indexName + ".stamps"

		var packagesFile, packagesFileGz, packagesFileStamps *os.File
		if packagesFile, err = ioutil.TempFile(cachePath, "Packages"); err != nil {
			log.Printf("error: can't create the Packages file in %q: %v\n", cachePath, err)
			return nil
		}
		defer packagesFile.Close()
		if packagesFileGz, err = os.Create(packagesFile.Name() + ".gz"); err != nil {
			log.Printf("error: can't create the Packages.gz file in %q: %v\n", cachePath, err)
			return nil
		}
		defer packagesFileGz.Close()

		if packagesFileStamps, err = os.Create(packagesFile.Name() + ".stamps"); err != nil {
			log.Printf("error: can't create the Packages.stamps file in %q: %v\n", cachePath, err)
			return nil
		}
		defer packagesFileStamps.Close()

		var index = scanner.packages.String()

		io.Copy(packagesFile, strings.NewReader(index))
		packagesFile.Sync()
		gzipper(packagesFileGz, strings.NewReader(index))
		packagesFileGz.Sync()
		scanner.packages.StampsTo(packagesFileStamps)
		packagesFileStamps.Sync()

		os.Remove(indexName)
		os.Remove(indexNameGz)
		os.Remove(indexNameStamps)
		os.Rename(packagesFile.Name(), indexName)
		os.Rename(packagesFileGz.Name(), indexNameGz)
		os.Rename(packagesFileStamps.Name(), indexNameStamps)

		return nil
	})
}

type packageScanner struct {
	packages *packageIndex
	nScanned int64
	nCached  int64

	root   string
	cache  string
	doSHA1 bool
	doMD5  bool
}

func (s *packageScanner) clear() {
	s.packages = &packageIndex{Entries: make(map[string]*ipkArchive)}
}

// scanPackages scans all files in root for packages. if it finds one, it
// checks the cache folder, it a cached version of the meta-information exists
// or if the meta-information is out-of-date. only if it's needed the meta
// information will be extracted from the packages and stored in the
// cache folder
func (s *packageScanner) scan(dirPath string, nworkers int) error {

	var dir, err = os.Open(dirPath)
	if err != nil {
		return fmt.Errorf("opening %q: %v\n", dirPath, err)
	}
	defer dir.Close()

	if err = s.provideCachePath(); err != nil {
		return err
	}

	var entries []os.FileInfo
	if entries, err = dir.Readdir(-1); err != nil {
		return fmt.Errorf("reading entries %q: %v\n", dirPath, err)
	}

	if s.packages == nil {
		s.clear()
	}
	var worker = newWorkerPool(nworkers)

	for _, entry := range entries {

		if s.skipNonPackage(entry) {
			continue
		}

		if s.fromCache(entry) {
			continue
		}

		worker.Hire()
		go s.scanAndCache(path.Join(dirPath, entry.Name()), worker)
	}
	worker.Wait()

	log.Printf("scanned %d packages (fresh %d|%d from cache) in %q.",
		s.packages.Len(), s.nScanned, s.nCached, s.root)

	return nil
}

func (s *packageScanner) scanAndCache(filePath string, worker *workerPool) {

	var n = time.Now()
	defer func() {
		worker.Release()
		log.Println("processed", filePath, time.Now().Sub(n))
	}()

	var archive, err = newIpkFromFile(filePath, s.doMD5, s.doSHA1)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}
	s.packages.Add(filepath.Base(filePath), archive)
	atomic.AddInt64(&s.nScanned, 1)

	if s.cache == "" {
		return
	}

	var cacheName = genCachedControlName(filepath.Base(filePath), s.cache)
	if err = ioutil.WriteFile(cacheName, []byte(archive.Control), 0644); err != nil {
		log.Println(cacheName, err)
	}
}

func (s *packageScanner) fromCache(entry os.FileInfo) bool {

	var controlName = genCachedControlName(entry.Name(), s.cache)
	var cacheEntry, err = os.Stat(controlName)
	if err != nil {
		return false
	}
	if entry.ModTime().Before(cacheEntry.ModTime()) {
		var archive, err = newIpkFromCache(entry.Name(), s.cache, s.doMD5, s.doSHA1)
		if err != nil {
			log.Printf("error: %v\n", err)
			return false
		}
		s.packages.Add(entry.Name(), archive)
		s.nCached += 1
		return true
	}
	return false
}

func (s *packageScanner) skipNonPackage(fi os.FileInfo) bool {
	return path.Ext(fi.Name()) != ".ipk" || fi.IsDir()
}

func (s *packageScanner) provideCachePath() error {
	if s.cache == "" {
		return nil
	}

	if err := os.MkdirAll(s.cache, 0755); err != nil {
		return fmt.Errorf("creating cache-folder %q: %v\n", s.cache, err)
	}
	return nil
}

func genCachedControlName(name, cache string) string {
	var cacheName = filepath.Join(cache, name)
	return cacheName + ".control"
}
