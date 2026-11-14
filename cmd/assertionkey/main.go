// Command assertionkey prints a new assertion signing key pair, one line each: "key=..." and "public=...".
// Scripts read it because macOS's openssl cannot easily make PKCS#8 and SPKI P-256 keys in base64.
package main

import (
	"fmt"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

func output() string {
	key, pub := authn.NewAssertionKeyPair()
	return fmt.Sprintf("key=%s\npublic=%s\n", key, pub)
}

func main() {
	fmt.Print(output())
}
