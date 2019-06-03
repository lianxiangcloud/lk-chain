package lklog

import (
	"fmt"
	"os"
	"strings"

	"github.com/astaxie/beego/logs"
)

type beelog struct {
	alarmlog *logs.BeeLogger
	datalog  *logs.BeeLogger
}

var lklog *beelog
var prefix string
var hostname string
var privZoneID int
var privRole string
var privModule string

const (
	AdapterConsole   = "console"
	AdapterFile      = "file"
	AdapterMultiFile = "multifile"
	AdapterMail      = "smtp"
	AdapterConn      = "conn"
	AdapterEs        = "es"
	AdapterJianLiao  = "jianliao"
	AdapterSlack     = "slack"
	AdapterAliLS     = "alils"
)

func init() {
	lklog = &beelog{alarmlog: logs.NewLogger(), datalog: logs.NewLogger()}
	lklog.alarmlog.SetLogger(AdapterConn, `{"net":"udp","addr":":10001","level":7,"prefix":"l_blockchain_2rd"}`)
	lklog.datalog.SetLogger(AdapterConn, `{"net":"udp","addr":":10001","level":7,"prefix":"l_blockchain_data"}`)

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "UNKNOW-HOST"
	}

	privRole = "UNKNOW-ROLE"
	privModule = "UNKNOW-MODULE"

	prefix = fmt.Sprintf("\x01%s\x01Zone-0\x01%s\x01%s\x01", hostname, privRole, privModule)
}

func formatLog(f interface{}, v ...interface{}) string {
	var msg string
	switch f.(type) {
	case string:
		msg = prefix + f.(string)
		if len(v) == 0 {
			return msg
		}
		if strings.Contains(msg, "%") && !strings.Contains(msg, "%%") {
			//format string
		} else {
			//do not contain format char
			msg += strings.Repeat(" %v", len(v))
		}
	default:
		msg = prefix + fmt.Sprint(f)
		if len(v) == 0 {
			return msg
		}
		msg += strings.Repeat(" %v", len(v))
	}
	return fmt.Sprintf(msg, v...)
}

func SetAlarmLogger(adapterName string, configs ...string) error {
	return lklog.alarmlog.SetLogger(adapterName, configs...)
}

func SetDataLogger(adapterName string, configs ...string) error {
	return lklog.datalog.SetLogger(adapterName, configs...)
}

func SetPrefix(role string, module string) {
	privRole = role
	privModule = module
	prefix = fmt.Sprintf("\x01%s\x01Zone-0\x01%s\x01%s\x01", hostname, privRole, privModule)
}

func Debug(f interface{}, v ...interface{}) {
	lklog.alarmlog.Debug(formatLog(f, v...))
}

func Info(f interface{}, v ...interface{}) {
	lklog.alarmlog.Info(formatLog(f, v...))
}

func Notice(f interface{}, v ...interface{}) {
	lklog.alarmlog.Notice(formatLog(f, v...))
}

func Warn(f interface{}, v ...interface{}) {
	lklog.alarmlog.Warn(formatLog(f, v...))
}

func Error(f interface{}, v ...interface{}) {
	lklog.alarmlog.Error(formatLog(f, v...))
}

func Critical(f interface{}, v ...interface{}) {
	lklog.alarmlog.Critical(formatLog(f, v...))
}

func Alert(f interface{}, v ...interface{}) {
	lklog.alarmlog.Alert(formatLog(f, v...))
}

func Emergency(f interface{}, v ...interface{}) {
	lklog.alarmlog.Emergency(formatLog(f, v...))
}

func InfoData(f interface{}, v ...interface{}) {
	lklog.datalog.Info(formatLog(f, v...))
}
