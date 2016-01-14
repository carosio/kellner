// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/textproto"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/blakesmith/ar"
)

type ipkArchive struct {
	Name     string
	Control  string // content of 'control' file
	Header   map[string]string
	FileInfo os.FileInfo
	Md5      string
	Sha1     string
}

// ControlToHeader parses 'control' and stores the result in ipkg.Header
func (ipk *ipkArchive) ControlToHeader(control string) error {
	reader := bufio.NewReader(strings.NewReader(control))
	protoReader := textproto.NewReader(reader)
	for {
		line, err := protoReader.ReadContinuedLine()
		if err == io.EOF {
			break
		}
		i := strings.IndexByte(line, ':')
		if i == -1 {
			return fmt.Errorf("invalid package-field %q", line)
		}

		ipk.Header[line[:i]] = strings.TrimSpace(line[i+1:])
	}
	return nil
}

func (ipk *ipkArchive) EnhanceHeader() {
	ipk.Header["Size"] = strconv.FormatInt(ipk.FileInfo.Size(), 10)
	if ipk.Md5 != "" {
		ipk.Header["MD5Sum"] = ipk.Md5
	}
	if ipk.Sha1 != "" {
		ipk.Header["SHA1"] = ipk.Sha1
	}
}

func (ipk *ipkArchive) DirEntry() dirEntry {

	descr := ipk.Header["Description"]
	if len(descr) > 64 {
		descr = descr[:64] + "..."
	}

	return dirEntry{
		Name:     ipk.Name,
		ModTime:  ipk.FileInfo.ModTime(),
		Size:     ipk.FileInfo.Size(),
		Descr:    descr,
		RawDescr: ipk.Header["Description"],
	}
}

// HeaderTo writes the package-header to 'to'.
// according to https://www.debian.org/doc/debian-policy/ch-controlfields.html
// the order of the fields does not matter
// according to https://wiki.debian.org/RepositoryFormat#A.22Packages.22_Indices
// 'Packages' should be the first field.
func (ipk *ipkArchive) HeaderTo(w io.Writer) {

	p, ok := ipk.Header["Package"]
	if ok {
		fmt.Fprintf(w, "Package: %s\n", p)
	}

	for key := range ipk.Header {
		if key != "Package" {
			fmt.Fprintf(w, "%s: %s\n", key, ipk.Header[key])
		}
	}
}

// ControlAndChecksumTo writes the 'control' file,
// the file name, the size and the checksums to the
// writer 'w'
func (ipk *ipkArchive) ControlAndChecksumTo(w io.Writer) {
	io.WriteString(w, ipk.Control)
	fmt.Fprintf(w, "Filename: %s\n", ipk.Name)
	fmt.Fprintf(w, "Size: %d\n", ipk.FileInfo.Size())
	if ipk.Md5 != "" {
		fmt.Fprintf(w, "MD5Sum: %s\n", ipk.Md5)
	}
	if ipk.Sha1 != "" {
		fmt.Fprintf(w, "SHA1: %s\n", ipk.Sha1)
	}
}

type ipkArchiveChan chan *ipkArchive

// extract 'control' file from 'reader'. the contents of a 'control' file
// is a set of key-value pairs as described in
// https://www.debian.org/doc/debian-policy/ch-controlfields.html
func extractControlFromIpk(reader io.Reader) (string, error) {

	var (
		arReader  *ar.Reader
		tarReader *tar.Reader
		gzReader  *gzip.Reader
	)

	arReader = ar.NewReader(reader)
	for {
		header, err := arReader.Next()
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("extracting contents: %v", err)
		} else if header == nil {
			break
		}

		// NOTE: strangeley the name of the files end with a "/" ... content error?
		if header.Name == "control.tar.gz/" || header.Name == "control.tar.gz" {
			gzReader, err = gzip.NewReader(arReader)
			if err != nil {
				return "", fmt.Errorf("analyzing control.tar.gz: %v", err)
			}
			break
		}
	}

	if gzReader == nil {
		return "", fmt.Errorf("missing control.tar.gz entry")
	}
	defer gzReader.Close()

	buffer := bytes.NewBuffer(nil)
	tarReader = tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("extracting control.tar.gz: %v", err)
		} else if header == nil {
			break
		}
		if header.Name != "./control" {
			continue
		}

		io.Copy(buffer, tarReader)
		break
	}

	if buffer.Len() == 0 {
		return "", fmt.Errorf("missing or empty 'control' file inside 'control.tar.gz'")
	}
	return buffer.String(), nil
}

func newIpkFromFile(name, root string, doMD5, doSHA1 bool) (*ipkArchive, error) {

	var (
		fullName = path.Join(root, name)
		file     *os.File
		writer   = make([]io.Writer, 0, 3)
		err      error
		md5er    hash.Hash
		sha1er   hash.Hash
	)

	file, err = os.Open(fullName)
	if err != nil {
		return nil, fmt.Errorf("openening %q: %v", fullName, err)
	}
	defer file.Close()

	writer = append(writer, ioutil.Discard)
	if doMD5 {
		md5er = md5.New()
		writer = append(writer, md5er)
	}
	if doSHA1 {
		sha1er = sha1.New()
		writer = append(writer, sha1er)
	}

	tee := io.TeeReader(file, io.MultiWriter(writer...))

	control, err := extractControlFromIpk(tee)
	if err != nil {
		return nil, fmt.Errorf("extract pkg-info from %q: %v", fullName, err)
	}

	archive := &ipkArchive{Name: name, Control: control, Header: make(map[string]string)}

	if err := archive.ControlToHeader(control); err != nil {
		return nil, fmt.Errorf("header parse error in %q: %v", fullName, err)
	}

	// consume the rest of the file to calculate md5/sha1
	io.Copy(ioutil.Discard, tee)
	file.Close() // close to free handles, 'collector' might block freeing otherwise

	archive.FileInfo, _ = os.Lstat(fullName)
	if md5er != nil {
		archive.Md5 = hex.EncodeToString(md5er.Sum(nil))
	}
	if sha1er != nil {
		archive.Sha1 = hex.EncodeToString(sha1er.Sum(nil))
	}

	return archive, nil
}

func newIpkFromCache(name, root string, doMD5, doSHA1 bool) (*ipkArchive, error) {

	var (
		fullName = genCachedControlName(name, root)
		control  []byte
		err      error
	)

	if control, err = ioutil.ReadFile(fullName); err != nil {
		return nil, fmt.Errorf("reading cache %q: %v\n", fullName, err)
	}

	var archive = &ipkArchive{Name: name,
		Control: string(control),
		Header:  make(map[string]string),
	}
	archive.FileInfo, _ = os.Stat(fullName)
	archive.ControlToHeader(archive.Control)

	return archive, nil
}
