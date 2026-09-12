package main

import (
	"bufio"
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protodelim"
	launchv1 "wails-spike/proto/launch/v1"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--auth-token-stdin" {
		fmt.Fprintln(os.Stderr, "secret or unexpected argument on command line")
		os.Exit(2)
	}
	message := new(launchv1.LaunchHandoff)
	if err := protodelim.UnmarshalFrom(bufio.NewReader(os.Stdin), message); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	// A second message is forbidden: EOF is the expected next byte.
	if _, err := os.Stdin.Read(make([]byte, 1)); err == nil {
		fmt.Fprintln(os.Stderr, "more than one handoff")
		os.Exit(4)
	}
	fmt.Println(message.GetBootstrap().GetRealmId())
}
