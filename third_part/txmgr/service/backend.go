package service

import (
	"errors"
	"fmt"
	"net"

	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

type RPCServer struct {
	wsNetwork  string
	wsEndPoint string
	wsListener net.Listener
	wsHandler  *rpc.Server
}

// network: tcp
// endPoint: 127.0.0.1:8010
// namespace: ethermint
func StartWSServer(network string, endPoint string, wsOrigins []string, apis []rpc.API) (*RPCServer, error) {
	if endPoint == "" {
		return nil, errors.New("StartWSServer: invalid endPoint")
	}
	// 1. New RPC Server
	handler := rpc.NewServer()

	// 2. New Custom Service && Register
	for _, api := range apis {
		if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
			log.Error("rpcServer RegisterName: failed", "err", err)
			return nil, err
		}
		log.Debug(fmt.Sprintf("WebSocket registered %T under '%s'", api.Service, api.Namespace))
	}

	// 3. New listener
	var (
		listener net.Listener
		err      error
	)
	if listener, err = net.Listen(network, endPoint); err != nil {
		log.Info("net.Listen: failed", "endPoint", endPoint, "err", err)
		return nil, err
	}

	// 4. Start WebSocket Server.
	log.Info("WebSocket endpoint opened:", "ws", listener.Addr())

	go rpc.NewWSServer(wsOrigins, handler).Serve(listener)

	return &RPCServer{
		wsEndPoint: endPoint,
		wsNetwork:  network,
		wsHandler:  handler,
		wsListener: listener,
	}, nil
}

func (s *RPCServer) Info() string {
	return fmt.Sprintf("Info: endpoint=%s, network=%s, listening=%b", s.wsEndPoint, s.wsNetwork, s.wsListener.Addr())
}

func (s *RPCServer) Stop() {
	if s.wsListener != nil {
		s.wsListener.Close()
		s.wsListener = nil
		log.Info(fmt.Sprintf("WebSocket endpoint closed: ws://%s", s.wsEndPoint))

	}
	if s.wsHandler != nil {
		s.wsHandler.Stop()
	}
}

func GetAPIs(eth *eth.Ethereum) ([]rpc.API, error) {
	service, err := NewService(eth)
	if err != nil {
		return nil, err
	}
	return []rpc.API{
		{
			Namespace: "ethermint",
			Version:   "1.0",
			Service:   service,
			Public:    true,
		},
	}, nil
}
