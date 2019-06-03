package reporttrans

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"third_part/lklog"
)

var lastDeliverTxTime = int64(0)
var transCount = 0
var warningInterval = 300

const (
	ERR_SUCC                   = 0
	ERR_HEURISTIC_LIMIT        = 1
	ERR_SIGN                   = 2
	ERR_NEGATIVE               = 3
	ERR_FROM_NOT_EXIST         = 4
	ERR_FROM_ZONE_NOT_LOC      = 5
	ERR_LISTENER_SIGN          = 6
	ERR_GAS_LIMIT              = 7
	ERR_NONCE                  = 8
	ERR_FROM_BALANCE           = 9
	ERR_GAS_CMP                = 10
	ERR_FROM_TO_ZONE_NOT_LOC   = 11
	ERR_FROM_LOCK              = 12
	ERR_UNDERPRICED            = 13
	ERR_SENDER_INVALID         = 14
	ERR_GAS_LIMIT_OR_GAS_PRICE = 15
	ERR_DELIVER_TX_SUCC        = 100
)

const (
	IdNodeUnconfirmedTxTooMany = 70002

	IdTendermintConsensusFailure = 70003
)

var (
	RateNodeUnconfirmedTxTooMany = 0.1
)

type TransactionReportData struct {
	Result      int
	From        string
	To          string
	Nonce       uint64
	Gas         string
	FromBalance string
	Cost        string
	Hash        string
}

func WarnJson(warnId int, a interface{}) {
	jsonData, err := json.Marshal(a)
	if err != nil {
		log.Printf("WarnJson: failed, err=%v", err)
	}
	log.Printf("WarnJson: warnId=%d, msg=%s\n", warnId, string(jsonData))
	lklog.Warn("%d\x01%s", warnId, string(jsonData))
}

func Report(data TransactionReportData) {
	str := fmt.Sprintf("20001\x01%v\x02%v\x02%v\x02%v\x02%v\x02%v\x02%v\x02%v\x02%v\x02%v", data.Result, data.From, data.To, 0, 0, data.Nonce, data.Gas, data.FromBalance, data.Cost, data.Hash)
	log.Printf("report TransactionReportData:%#v\n", data)
	log.Println(str)

	newlogstr := fmt.Sprintf("20001\x01Result,From,To,FromZone,ToZone,Nonce,Gas,FromBalance,Cost,Hash\x01%v\x01%v\x01%v\x01%v\x01%v\x01%v\x01%v\x01%v\x01%v\x01%v", data.Result, data.From, data.To, 0, 0, data.Nonce, data.Gas, data.FromBalance, data.Cost, data.Hash)
	lklog.InfoData(newlogstr)
	if data.Result != ERR_SUCC && data.Result != ERR_DELIVER_TX_SUCC {
		lklog.Info(str)
	}
}

func ReportNoDelverTx() {
	str := fmt.Sprintf("20002\x01%v\x02%v\x02%v", warningInterval, lastDeliverTxTime, transCount)
	log.Println("ReportNoDelverTx interval:", warningInterval, " lastDeliverTxTime:", lastDeliverTxTime, " count:", transCount)
	lklog.Info(str)
}

func UpdateTrans() {
	transCount++
	if lastDeliverTxTime == 0 {
		lastDeliverTxTime = time.Now().Unix()
		log.Println("first set last deliver tx time:", lastDeliverTxTime)
	}
}

func Check() {
	for {

		tm := time.Now().Unix()
		if ((tm - lastDeliverTxTime) > int64(warningInterval)) && (transCount > 0) {
			ReportNoDelverTx()
		}
		time.Sleep(time.Duration(warningInterval) * time.Second)
	}
}
func SetWarnInterval(seconds int) {
	warningInterval = seconds
}

func SetTxsWarnRate(rate float64) {
	RateNodeUnconfirmedTxTooMany = rate
}

func UpdateDeliverTx() {
	transCount = 0
	lastDeliverTxTime = time.Now().Unix()
}

func RunCheck() {
	go Check()

}
