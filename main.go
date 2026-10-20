package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/dandygardad/lazytfs/internal/tui"
)

// version is set at build time for releases: go build -ldflags "-X main.version=1.1.0"
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		// "go install ...@v1.1.0" builds have no ldflags, but Go records the module version
		if info, ok := debug.ReadBuildInfo(); ok && version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		fmt.Println("lazytfs " + version)
		return
	}

	app := tui.NewApp()
	if err := app.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
