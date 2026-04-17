// Command keygen generates a cryptographically random 32-byte AES key
// and prints it in the format accepted by the `ENC_KEYS` environment
// variable. The actual generation logic lives in internal/runtime so it
// is test-covered; main here is a thin CLI wrapper.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/eaglepoint/oops/backend/internal/runtime"
)

func main() {
	version := flag.Int("version", 1, "key version number")
	flag.Parse()
	line, err := runtime.GenerateKeyLine(*version, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(line)
}
