package events

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

var gEventSystem EventSystem

func GetEventSystem() *EventSystem {
	return &gEventSystem
}

type EventSystem struct {
	commitBlockFeed event.Feed
	scope           event.SubscriptionScope
}

func NewEventSystem() *EventSystem {
	return &EventSystem{
		commitBlockFeed: event.Feed{},
		scope:           event.SubscriptionScope{},
	}
}

type CommitBlockEvent struct {
	Hash common.Hash
}

func (es *EventSystem) SubscribeCommitBlockEvent(ch chan<- CommitBlockEvent) event.Subscription {
	return es.scope.Track(es.commitBlockFeed.Subscribe(ch))
}

//SendCommitBlockEvent: once we receive  a commitBlockEvent, we should
func (es *EventSystem) SendCommitBlockEvent(e CommitBlockEvent) int {
	result := es.commitBlockFeed.Send(e)
	log.Info("[lkrpc] sendCommitBlockEvent:", "result", result)
	return result
}

func (es *EventSystem) Stop() {
	es.scope.Close()
}
