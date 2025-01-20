package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/kitproj/kubeauto/internal"
	"os"
	"os/signal"
	"runtime/debug"
)

func main() {
	var namespace, labels, group string
	var hostPortOffset int
	var help bool
	var version bool
	flag.StringVar(&namespace, "n", "", "namespace to filter resources, defaults to the current namespace ")
	flag.StringVar(&labels, "l", "", "comma separated list of labels to filter resources, e.g. app=nginx, defaults to all resources")
	flag.StringVar(&group, "g", "", "the group to watch, defaults to core resources")
	flag.IntVar(&hostPortOffset, "p", 0, "the offset to add to the host port, e.g. if the container listens on 8080 and the host port is 30000, the offset is 38080, defaults to 0")
	flag.BoolVar(&help, "h", false, "print help")
	flag.BoolVar(&version, "v", false, "print version")
	flag.Parse()

	if help {
		flag.Usage()
		return
	}

	if version {
		info, _ := debug.ReadBuildInfo()
		fmt.Println(info.Main.Version)
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := internal.Run(ctx, group, namespace, labels, hostPortOffset); err != nil {
		panic(err)
	}
}
