package main

import (
	"bytes"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/x509/pkix"
	"fmt"
	"strings"
)

func clientIdByName(name *pkix.Name) string {

	buf := bytes.NewBuffer(nil)

	write := func(key, val string, prepend_comma bool) {
		if val == "" {
			return
		}
		if prepend_comma {
			fmt.Fprint(buf, ",")
		}
		fmt.Fprintf(buf, "%s=%s", key, val)
	}

	write("CN", name.CommonName, false)
	write("O", strings.Join(name.Organization, "_"), buf.Len() > 0)
	write("OU", strings.Join(name.OrganizationalUnit, "_"), buf.Len() > 0)
	write("O", strings.Join(name.Country, "_"), buf.Len() > 0)
	write("O", strings.Join(name.Locality, "_"), buf.Len() > 0)

	return buf.String()
}
