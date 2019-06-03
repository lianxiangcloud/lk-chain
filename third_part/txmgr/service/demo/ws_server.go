package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

type Service struct {
}

type Args struct {
	S string
}

type Result struct {
	String string
	Int    int
	Args   *Args
}

func (s *Service) Echo(str string, i int, args *Args) Result {
	return Result{str, i, args}
}

func (s *Service) SomeSubs(ctx context.Context, n int) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	subscription := notifier.CreateSubscription()

	go func() {
		go func() {
			// wait until client has received the Subscribe response.
			time.Sleep(5 * time.Second)
			result := Result{
				String: "notify-string",
				Args:   &Args{"notify-args"},
			}

			// Notify.
			for i := 0; i < n; i++ {
				result.Int = i
				if err := notifier.Notify(subscription.ID, &result); err != nil {
					fmt.Printf("notifier.Notify %dth fail: %s\n", i, err)
					return
				}
				fmt.Printf("notifier.Notify %dth ok\n", i)
				time.Sleep(time.Second * 1)
			}
		}()

		select {
		case <-notifier.Closed():
			fmt.Printf("notifier.Closed\n")
		case err := <-subscription.Err():
			if err != nil {
				fmt.Printf("subscription.Err: %s", err)
			} else {
				fmt.Printf("subscription exit\n")
			}
		}
	}()

	fmt.Printf("SomeSubs: id=%s\n", subscription.ID)
	return subscription, nil
}

func main() {
	endpoint := "127.0.0.1:8010"
	var wsOrigins []string

	// 1. New RPC Server
	rpcServ := rpc.NewServer()

	// 2. New Custom Service && Register
	service := new(Service)
	if err := rpcServ.RegisterName("test", service); err != nil {
		fmt.Printf("rpcServer RegisterName fail: %s\n", err)
		return
	}

	// 3. New listener
	var listener net.Listener
	var err error
	if listener, err = net.Listen("tcp", endpoint); err != nil {
		fmt.Printf("net.Listen %s fail: %s\n", endpoint, err)
		return
	}

	// 4. Start WebSocket Server.
	fmt.Printf("WebSocket endpoint opened: ws://%s\n", listener.Addr())
	err = rpc.NewWSServer(wsOrigins, rpcServ).Serve(listener)
	fmt.Printf("WebSocket exit: %s\n", err)
}
