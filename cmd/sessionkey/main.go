// Command sessionkey prints a new session signing key pair, one line each: "seed=..." and "public=...".
// Scripts read it because macOS's openssl cannot make Ed25519 keys.
package main

import (
	"fmt"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

func main() {
	seed, pub := authn.NewKeyPair()
	fmt.Printf("seed=%s\npublic=%s\n", seed, pub)
}
