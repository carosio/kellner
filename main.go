// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

// *kellner* scans package files in a given directory
// and builds a Packages.gz file on the fly. it then serves the
// Packages.gz and the .ipk files by the built-in httpd
// and is ready to be used from opkg
//
// related tools:
// * https://github.com/17twenty/opkg-scanpackages
// * opkg-make-index from the opkg-utils collection

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

const VERSION = "kellner-0.2"

func main() {

	var (
		nworkers        = flag.Int("workers", 4, "number of workers")
		bind            = flag.String("bind", ":8080", "address to bind to")
		rootName        = flag.String("root", "", "directory containing the packages")
		dumpPackageList = flag.Bool("dump", false, "just dump the package list and exit")
		addMd5          = flag.Bool("md5", true, "calculate md5 of scanned packages")
		addSha1         = flag.Bool("sha1", false, "calculate sha1 of scanned packages")
		useGzip         = flag.Bool("gzip", true, "use 'gzip' to compress the package index. if false: use golang")
		showVersion     = flag.Bool("version", false, "show version")

		sslKey               = flag.String("ssl-key", "", "PEM encoded ssl-key")
		sslCert              = flag.String("ssl-cert", "", "PEM encoded ssl-cert")
		sslRequireClientCert = flag.Bool("require-client-cert", false, "require a client-cert")
		sslClientIdMuxRoot   = flag.String("client-map", "", "directory containing the client-mappings")

		listen net.Listener
	)

	flag.Parse()

	if *showVersion {
		fmt.Println(VERSION)
		return
	}

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind\n")
		os.Exit(1)
	}

	if *rootName == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root")
	}
	*rootName, _ = filepath.Abs(*rootName)

	// simple use-case: scan one directory and dump the created
	// packages-list to stdout.
	if *dumpPackageList {
		now := time.Now()
		log.Println("start building index from", *rootName)

		packages, err := ScanDirectoryForPackages(*rootName, *nworkers, *addMd5, *addSha1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
		log.Println("done building index")
		log.Printf("time to parse %d packages: %s\n", len(packages.Entries), time.Since(now))

		os.Stdout.WriteString(packages.String())
		return
	}

	// regular use-case: serve the given directory + the Packages file(s)
	// recursively.
	//
	// setup the listener: either ssl or pure tcp
	l, err := net.Listen("tcp", *bind)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: binding to %q failed: %v\n", *bind, err)
		os.Exit(1)
	}
	listen = l

	if *sslCert != "" || *sslKey != "" {

		cert, err := tls.LoadX509KeyPair(*sslCert, *sslKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: loading x509-keypair from %q - %q failed: %v\n", *sslCert, *sslKey, err)
			os.Exit(2)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			// disable ssl3 and tls1.0 (protect against beast, poodle etc)
			MinVersion: tls.VersionTLS11,
			NextProtos: []string{"http/1.1"},
			// avoid rc4
			CipherSuites: []uint16{
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
		}
		if *sslRequireClientCert {
			tlsConfig.ClientAuth = tls.RequireAnyClientCert
		}

		listen = tls.NewListener(listen, tlsConfig)
	}

	log.Println("listen on", listen.Addr())

	gzipper := Gzipper(GzGzipPipe)
	if !*useGzip {
		gzipper = GzGolang
	}

	// the root-muxer is used either directly (non-ssl-client-cert case) or
	// as a lookup-pool for ClientIdMuxer to get the real worker
	rootMuxer := http.NewServeMux()

	startTime := time.Now()
	indices := make([]string, 0)
	filepath.Walk(*rootName, func(path string, fi os.FileInfo, err error) error {

		if !fi.IsDir() {
			return nil
		}

		var (
			packages *PackageIndex
			now      = time.Now()
		)

		log.Printf("start building index for %q", path)

		if packages, err = ScanDirectoryForPackages(path, *nworkers, *addMd5, *addSha1); err != nil {
			log.Printf("error: %v", err)
			return nil
		}

		log.Printf("done building index for %q", path)
		log.Printf("time to parse %d packages in %q: %s\n", len(packages.Entries), path, time.Since(now))

		muxPath := path[len(*rootName):]
		if muxPath == "" {
			muxPath = "/"
		}

		// non-package directories
		if len(packages.Entries) == 0 {
			rootMuxer.Handle(muxPath, http.FileServer(http.Dir(path)))
			return nil
		}

		AttachHttpHandler(rootMuxer, packages, muxPath, *rootName, gzipper)

		indices = append(indices, muxPath)

		return nil
	})
	// TODO: this is specific to non-client-id situations
	AttachOpkgRepoSnippet(rootMuxer, "/opkg.conf", indices)

	log.Println()
	log.Printf("processed %d package-folders in %s", len(indices), time.Since(startTime))

	var httpHandler http.Handler = rootMuxer
	if *sslClientIdMuxRoot != "" {
		httpHandler = &ClientIdMuxer{
			IdRoot:    *sslClientIdMuxRoot,
			RootMuxer: rootMuxer,
		}
	}

	httpHandler = logRequests(httpHandler)

	log.Println()
	proto := "http://"
	if *sslKey != "" {
		proto = "https://"
	}
	log.Printf("serving at %s", proto+listen.Addr().String())
	http.Serve(listen, httpHandler)
}

func ScanDirectoryForPackages(dir string, nworkers int, addMd5, addSha1 bool) (*PackageIndex, error) {

	root, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("opening -root %q: %v\n", dir, err)
	}

	entries, err := root.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("reading dir entries from -root %q: %v\n", dir, err)
	}

	packages := &PackageIndex{Entries: make(map[string]*Ipkg)}
	workers := NewWorkerPool(nworkers)

	for _, entry := range entries {
		if path.Ext(entry) != ".ipk" {
			continue
		}
		workers.Hire()
		go func(name string) {
			defer workers.Release()
			ipkg, err := NewIpkgFromFile(name, dir, addMd5, addSha1)
			if err != nil {
				log.Printf("error: %v\n", err)
				return
			}
			packages.Lock()
			packages.Entries[name] = ipkg
			packages.Unlock()
		}(entry)
	}
	workers.Wait()
	return packages, nil
}

type WorkerPool struct {
	sync.WaitGroup
	worker chan bool
}

func NewWorkerPool(n int) *WorkerPool {
	return &WorkerPool{worker: make(chan bool, n)}
}

// hire / block a worker from the pool
func (pool *WorkerPool) Hire() {
	pool.worker <- true
	pool.Add(1)
}

// release / unblock a blocked worker from the pool
func (pool *WorkerPool) Release() {
	pool.Done()
	<-pool.worker
}
