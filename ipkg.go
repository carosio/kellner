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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/blakesmith/ar"
)

type Ipkg struct {
	Name     string
	Control  string // content of 'control' file
	Header   map[string]string
	FileInfo os.FileInfo
	Md5      string
	Sha1     string
}

// parses 'control' and stores the result in ipkg.Header
func (ipkg *Ipkg) ControlToHeader(control string) error {
	reader := bufio.NewReader(strings.NewReader(control))
	proto_reader := textproto.NewReader(reader)
	for {
		line, err := proto_reader.ReadContinuedLine()
		if err == io.EOF {
			break
		}
		i := strings.IndexByte(line, ':')
		if i == -1 {
			return fmt.Errorf("invalid package-field %q", line)
		}

		ipkg.Header[line[:i]] = strings.TrimSpace(line[i+1:])
	}
	return nil
}

func (ipkg *Ipkg) EnhanceHeader() {
	ipkg.Header["Size"] = strconv.FormatInt(ipkg.FileInfo.Size(), 10)
	if ipkg.Md5 != "" {
		ipkg.Header["MD5Sum"] = ipkg.Md5
	}
	if ipkg.Sha1 != "" {
		ipkg.Header["SHA1"] = ipkg.Sha1
	}
}

func (ipkg *Ipkg) DirEntry() DirEntry {

	descr := ipkg.Header["Description"]
	if len(descr) > 64 {
		descr = descr[:64] + "..."
	}

	return DirEntry{
		Name:    ipkg.Name,
		ModTime: ipkg.FileInfo.ModTime(),
		Size:    ipkg.FileInfo.Size(),
		Descr:   descr,
	}
}

// according to https://www.debian.org/doc/debian-policy/ch-controlfields.html
// the order of the fields does not matter
// according to https://wiki.debian.org/RepositoryFormat#A.22Packages.22_Indices
// 'Packages' should be the first field.
func (ipkg *Ipkg) HeaderTo(w io.Writer) {

	p, ok := ipkg.Header["Package"]
	if ok {
		fmt.Fprintf(w, "Package: %s\n", p)
	}

	for key := range ipkg.Header {
		if key != "Package" {
			fmt.Fprintf(w, "%s: %s\n", key, ipkg.Header[key])
		}
	}
}

func (ipkg *Ipkg) ControlAndChecksumTo(w io.Writer) {
	io.WriteString(w, ipkg.Control)
	fmt.Fprintf(w, "Filename: %s\n", ipkg.Name)
	fmt.Fprintf(w, "Size: %d\n", ipkg.FileInfo.Size())
	if ipkg.Md5 != "" {
		fmt.Fprintf(w, "MD5Sum: %s\n", ipkg.Md5)
	}
	if ipkg.Sha1 != "" {
		fmt.Fprintf(w, "SHA1: %s\n", ipkg.Sha1)
	}
}

type IpkgChan chan *Ipkg

type PackageIndex struct {
	sync.Mutex
	Entries map[string]*Ipkg
}

func (pi *PackageIndex) StringTo(w io.Writer) {
	for _, name := range pi.SortedNames() {
		entry := pi.Entries[name]
		entry.ControlAndChecksumTo(w)
		fmt.Fprintln(w)
	}
}

func (pi *PackageIndex) StampsTo(w io.Writer) {
	for _, name := range pi.SortedNames() {
		entry := pi.Entries[name]
		fmt.Fprintf(w, "%d %s\n", entry.FileInfo.ModTime().Unix(), name)
	}
}

func (pi *PackageIndex) String() string {
	buf := bytes.NewBuffer(nil)
	pi.StringTo(buf)
	return buf.String()
}

func (pi *PackageIndex) SortedNames() []string {
	var (
		names = make([]string, len(pi.Entries))
		i     int
	)
	for name := range pi.Entries {
		names[i] = name
		i++
	}
	sort.Strings(names)
	return names
}

// extract 'control' file from 'reader'. the contents of a 'control' file
// is a set of key-value pairs as described in
// https://www.debian.org/doc/debian-policy/ch-controlfields.html
func ExtractControlFromIpk(reader io.Reader) (string, error) {

	var (
		ar_reader  *ar.Reader
		tar_reader *tar.Reader
		gz_reader  *gzip.Reader
	)

	ar_reader = ar.NewReader(reader)
	for {
		header, err := ar_reader.Next()
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("extracting contents: %v", err)
		} else if header == nil {
			break
		}

		// NOTE: strangeley the name of the files end with a "/" ... content error?
		if header.Name == "control.tar.gz/" || header.Name == "control.tar.gz" {
			gz_reader, err = gzip.NewReader(ar_reader)
			break
		}
	}

	if gz_reader == nil {
		return "", fmt.Errorf("missing control.tar.gz file")
	}
	defer gz_reader.Close()

	buffer := bytes.NewBuffer(nil)
	tar_reader = tar.NewReader(gz_reader)
	for {
		header, err := tar_reader.Next()
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("extracting control.tar.gz: %v", err)
		} else if header == nil {
			break
		}
		if header.Name != "./control" {
			continue
		}

		io.Copy(buffer, tar_reader)
		break
	}

	if buffer.Len() == 0 {
		return "", fmt.Errorf("missing or empty 'control' file inside 'control.tar.gz'")
	}
	return buffer.String(), nil
}

func NewIpkgFromFile(name, root string, do_md5, do_sha1 bool) (*Ipkg, error) {

	var (
		full_name = path.Join(root, name)
		file      *os.File
		writer    []io.Writer = make([]io.Writer, 0, 3)
		err       error
		md5er     hash.Hash
		sha1er    hash.Hash
	)

	file, err = os.Open(full_name)
	if err != nil {
		return nil, fmt.Errorf("openening %q: %v", full_name, err)
	}
	defer file.Close()

	writer = append(writer, ioutil.Discard)
	if do_md5 {
		md5er = md5.New()
		writer = append(writer, md5er)
	}
	if do_sha1 {
		sha1er = sha1.New()
		writer = append(writer, sha1er)
	}

	tee := io.TeeReader(file, io.MultiWriter(writer...))

	control, err := ExtractControlFromIpk(tee)
	if err != nil {
		return nil, fmt.Errorf("error: extract pkg-info from %q: %v", full_name, err)
	}

	ipkg := &Ipkg{Name: name, Control: control, Header: make(map[string]string)}

	if err := ipkg.ControlToHeader(control); err != nil {
		return nil, fmt.Errorf("error: header parse error in %q: %v", full_name, err)
	}

	// consume the rest of the file to calculate md5/sha1
	io.Copy(ioutil.Discard, tee)
	file.Close() // close to free handles, 'collector' might block freeing otherwise

	ipkg.FileInfo, _ = os.Lstat(full_name)
	if md5er != nil {
		ipkg.Md5 = hex.EncodeToString(md5er.Sum(nil))
	}
	if sha1er != nil {
		ipkg.Sha1 = hex.EncodeToString(sha1er.Sum(nil))
	}

	return ipkg, nil
}
