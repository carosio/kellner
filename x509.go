package main

import (
	"bytes"
	"crypto/x509/pkix"
	"fmt"
	"io"

	// enforce linking of several crypto-hashes
	_ "crypto/sha256"
	_ "crypto/sha512"
)

// returns a "normalized" variant of the pkix.Name
// which might be used as a file on disk
func clientIdByName(name *pkix.Name) string {

	nameBytes := pkixNameToBytes(name, true)
	return string(nameBytes)
}

// lookup table. it's a short table, we just scan it linear in pkixNameToBytes()
// the values are taken from http://golang.org/src/crypto/x509/pkix/pkix.go, see
// oidCountry, oidCommonName etc.
var oidToKeys = [...]struct {
	oid [4]int
	key string
}{
	{[4]int{2, 5, 4, 6}, "C"},   // country
	{[4]int{2, 5, 4, 10}, "O"},  // organization
	{[4]int{2, 5, 4, 11}, "OU"}, // organizational unit
	{[4]int{2, 5, 4, 3}, "CN"},  // common name
	{[4]int{2, 5, 4, 5}, "SN"},  // serial number
	{[4]int{2, 5, 4, 7}, "L"},   // locality
	{[4]int{2, 5, 4, 8}, "P"},   // province
	{[4]int{2, 5, 4, 9}, "S"},   // street // TODO: check correct key
	{[4]int{2, 5, 4, 17}, "PC"}, // postal code // TODO: check correct key
}

// returns a serialized version of 'name' for each known
// asn1.object-identifier ("key") in the form:
//  key1=value1,key2=value2,key3=value3...
//
func pkixNameToBytes(name *pkix.Name, cleanValues bool) []byte {

	buf := bytes.NewBuffer(nil)
	for i := range name.Names {

		entry := &name.Names[i]
		key := ""
		for j := 0; j < len(oidToKeys); j++ {
			if len(entry.Type) == 4 &&
				entry.Type[0] == oidToKeys[j].oid[0] &&
				entry.Type[1] == oidToKeys[j].oid[1] &&
				entry.Type[2] == oidToKeys[j].oid[2] &&
				entry.Type[3] == oidToKeys[j].oid[3] {

				key = oidToKeys[j].key
				break
			}
		}

		if key == "" {
			continue
		}

		if buf.Len() > 0 {
			io.WriteString(buf, ",")
		}

		oldLen := len(buf.Bytes())
		fmt.Fprintf(buf, "%s=%s", key, entry.Value)
		if cleanValues {
			cleanPkixNameBytes(buf.Bytes()[oldLen+len(key)+1:])
		}
	}

	return buf.Bytes()
}

// replace any [^a-zA-Z0-9_-] by a '_'
func cleanPkixNameBytes(in []byte) {
	for i := range in {
		c := in[i]
		switch {
		default:
			in[i] = '_'
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'z':
		case 'A' <= c && c <= 'Z':
		case c == '_', c == '-':
		}
	}
}
