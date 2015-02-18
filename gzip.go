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
	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	gz.Close()
	return nil
}

// use a pipe to 'gzip' to create the gz in a way
// that opkg accepts the output. right now it's unclear
// why opkg explodes when it hits a golang-native-created
// gz file.
func GzGzipPipe(w io.Writer, r io.Reader) error {
	cmd := exec.Command("gzip", "-9", "-c")
	cmd.Stdin = r
	cmd.Stdout = w
	return cmd.Run()
}
