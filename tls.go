// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"

	// enforce linking of several crypto-hashes
	_ "crypto/sha256"
	_ "crypto/sha512"
)

type tlsOptions struct {
	keyFileName       string
	certFileName      string
	clientCasFileName string
	requireClientCert bool
}

func initTLS(listener net.Listener, opts *tlsOptions) (net.Listener, error) {

	cert, err := tls.LoadX509KeyPair(opts.certFileName, opts.keyFileName)
	if err != nil {
		return listener, fmt.Errorf("loading x509-keypair from %q - %q failed: %v", opts.certFileName, opts.keyFileName, err)
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

	if opts.clientCasFileName != "" {
		certBytes, err := ioutil.ReadFile(opts.clientCasFileName)
		if err != nil {
			return listener, fmt.Errorf("loading ca-certs from %q failed: %v", opts.clientCasFileName, err)
		}

		tlsConfig.ClientCAs = x509.NewCertPool()
		if ok := tlsConfig.ClientCAs.AppendCertsFromPEM(certBytes); !ok {
			return listener, fmt.Errorf("adding ca-certs from %q to the pool failed", opts.clientCasFileName)
		}

		log.Printf("added %d certs from %q to ca-certs", len(tlsConfig.ClientCAs.Subjects()), opts.clientCasFileName)
	}

	if opts.requireClientCert {
		tlsConfig.ClientAuth = tls.RequireAnyClientCert

		// user gave a list of client-cas. this indicates that she wants
		// to check the http-client-certs
		if tlsConfig.ClientCAs != nil {
			log.Println("tls.RequireAndVerifyClientCert")
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
	}

	return tls.NewListener(listener, tlsConfig), nil
}
