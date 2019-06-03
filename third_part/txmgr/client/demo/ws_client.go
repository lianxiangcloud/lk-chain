package main

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

type Args struct {
	S string
}

type Result struct {
	String string
	Int    int
	Args   *Args
}

func main() {
	// 1. Connect to WebSocket Server.
	client, err := rpc.Dial("ws://127.0.0.1:8010")
	if err != nil {
		fmt.Printf("rpc.Dial fail: %s\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 2. new Channel to receive notifi.
	subch := make(chan Result)

	// 3. Subscribe
	n := 5
	cliSubscription, err := client.Subscribe(ctx, "ethermint", subch, "blockSubscribe")
	if err != nil {
		fmt.Printf("client.Subscribe fail: %s\n", err)
		return
	}

	// 4. for loop to receive notifi and handle it.
	nums := 0
	for nums < n {
		select {
		case result := <-subch: // got notifi
			fmt.Printf("got %dth notifier: %s, %d, %s\n", nums, result.String, result.Int, result.Args.S)
			nums++
		case err := <-cliSubscription.Err(): // got error
			fmt.Printf("Subscription got err: %s\n", err)
		}
	}

	// 5. When we no need to notifi, call Unsubscribe().
	fmt.Printf("Client UnSubscribe\n")
	cliSubscription.Unsubscribe()

	// -------------------------------------------
	// Test for Method_Call
	fmt.Printf("call test_echo begin\n")
	resp := Result{}
	err = client.Call(&resp, "test_echo", "hello world", 10, &Args{"test-args"})
	if err != nil {
		fmt.Printf("Call test_echo fail: %s\n", err)
		return
	}

	if !reflect.DeepEqual(resp, Result{"hello world", 10, &Args{"test-args"}}) {
		fmt.Printf("incorrect result %#v\n", resp)
		return
	}
	fmt.Printf("call test_echo ok\n")
}
