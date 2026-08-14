package main

import (
	"fmt"
	"log"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
)

func main() {
	app, err := loader.LoadDir("/Users/dmy/work/qorm/examples/mario")
	if err != nil {
		log.Fatal(err)
	}
	rt := runtime.New(app)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Entry scene:", app.Entry)
	fmt.Println("Current scene:", rt.CurrentScene())
	fmt.Println("SceneKeys:", app.SceneKeys)
	fmt.Println("SceneKeyReleases:", app.SceneKeyReleases)
}
