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
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

const versionString = "kellner-0.5.2"

func main() {

	var (
		dumpPackageList = flag.Bool("dump", false, "just dump the package list and exit")
		prepareCache    = flag.Bool("prep-cache", false, "scan all packages and prepare the cache folder, do not serve anything")

		bind        = flag.String("bind", ":8080", "address to bind to")
		rootName    = flag.String("root", "", "directory containing the packages")
		cacheName   = flag.String("cache", "cache", "directory containing cached meta-files (eg. control)")
		nworkers    = flag.Int("workers", 4, "number of workers")
		addMd5      = flag.Bool("md5", true, "calculate md5 of scanned packages")
		addSha1     = flag.Bool("sha1", false, "calculate sha1 of scanned packages")
		useGzip     = flag.Bool("gzip", true, "use 'gzip' to compress the package index. if false: use golang")
		showVersion = flag.Bool("version", false, "show version and exit")
		logFileName = flag.String("log", "", "log to given filename")

		tlsKey               = flag.String("tls-key", "", "PEM encoded ssl-key")
		tlsCert              = flag.String("tls-cert", "", "PEM encoded ssl-cert")
		tlsClientCas         = flag.String("tls-client-ca-file", "", "file with PEM encoded list of ssl-certs containing the CAs")
		tlsRequireClientCert = flag.Bool("require-client-cert", false, "require a client-cert")
		tlsClientIDMuxRoot   = flag.String("idmap", "", "directory containing the client-mappings")
		printClientCert      = flag.String("print-client-cert-id", "", "print client-id for given .cert and exit")

		listen net.Listener
		err    error
	)

	flag.Parse()

	if *showVersion {
		fmt.Println(versionString)
		return
	}

	if *printClientCert != "" {
		if err = printClientIDTo(os.Stdout, *printClientCert); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind\n")
		os.Exit(1)
	}

	if *rootName == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root\n")
		os.Exit(1)
	}
	*rootName, _ = filepath.Abs(*rootName)

	setupLogging(*logFileName)

	// simple use-case: scan one directory and dump the created
	// packages-list to stdout.
	if *dumpPackageList {
		dumpPackages(*rootName, *nworkers, *addMd5, *addSha1)
		return
	}

	if *cacheName == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -cache\n")
		os.Exit(1)
	}
	*cacheName, _ = filepath.Abs(*cacheName)

	if err = os.MkdirAll(*cacheName, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating %q: %v\n", *cacheName, err)
		os.Exit(1)
	}

	gzipper := gzWrite(gzGzipPipe)
	if !*useGzip {
		gzipper = gzGolang
	}

	scanRoot(*rootName, *cacheName, *nworkers, *addMd5, *addSha1, gzipper)

	if *prepareCache {
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

	if *tlsCert != "" || *tlsKey != "" {

		var tlsOpts = tlsOptions{
			keyFileName:       *tlsKey,
			certFileName:      *tlsCert,
			requireClientCert: *tlsRequireClientCert,
			clientCasFileName: *tlsClientCas,
		}

		if listen, err = initTLS(listen, &tlsOpts); err != nil {

			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
	}

	go rescan(*rootName, *cacheName, *nworkers, *addMd5, *addSha1, gzipper)

	log.Println("listen on", listen.Addr())

	// the root-muxer is used either directly (non-ssl-client-cert case) or
	// as a lookup-pool for ClientIdMuxer to get the real handler
	var rootMuxer = http.NewServeMux()
	rootMuxer.Handle("/", makeIndexHandler(*rootName, *cacheName))

	var httpHandler http.Handler = rootMuxer
	if *tlsClientIDMuxRoot != "" {
		httpHandler = &clientIDMuxer{
			Folder: *tlsClientIDMuxRoot,
			Muxer:  rootMuxer,
		}
	}

	httpHandler = logRequests(httpHandler)

	log.Println()
	proto := "http://"
	if *tlsKey != "" {
		proto = "https://"
	}
	log.Printf("serving at %s", proto+listen.Addr().String())
	http.Serve(listen, httpHandler)
}
