package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"encoding/json"
	"sync/atomic"
	"path/filepath"

	"github.com/go-kit/kit/log/term"
	"github.com/go-logfmt/logfmt"
	tmlog "github.com/tendermint/tmlibs/log"
	kitlog "github.com/go-kit/kit/log"
	kitlevel "github.com/go-kit/kit/log/level"
)

const (
	msgKey     = "_msg" // "_" prefixed to avoid collisions
	moduleKey  = "module"

	bufferSize = 16 * 1024
	timeFormat = "2006-01-02 15:04:05.000"
)

var (
	flushNever = 59 * time.Minute
	flushOften = 2311 * time.Millisecond
	cstZone    = time.FixedZone("CST", 8*3600)
)

type rotateLogger struct {
	srcLogger  kitlog.Logger
	baseLogger BaseLogger
}

// Interface assertions
var _ tmlog.Logger = (*rotateLogger)(nil)

// NewRotateLogger returns a logger that encodes msg and keyvals to the Writer
// using go-kit's log as an underlying logger and our custom formatter. Note
// that underlying logger could be swapped with something else.
func NewRotateLogger(path string) tmlog.Logger {
	// Color by level value
	colorFn := func(keyvals ...interface{}) term.FgBgColor {
		if keyvals[0] != kitlevel.Key() {
			panic(fmt.Sprintf("expected level key to be first, got %v", keyvals[0]))
		}
		switch keyvals[1].(kitlevel.Value).String() {
		case "debug":
			return term.FgBgColor{Fg: term.DarkGray}
		case "error":
			return term.FgBgColor{Fg: term.Red}
		default:
			return term.FgBgColor{}
		}
	}

	lg := newFileWriter()
	err := lg.Init(nil, path)
	if err != nil {
		panic(fmt.Sprintf("NewRotateLogger failed"))
	}

	return &rotateLogger{term.NewLogger(lg, NewTMFmtLogger, colorFn), lg}
}

// Info logs a message at level Info.
func (l *rotateLogger) Info(msg string, keyvals ...interface{}) {
	lWithLevel := kitlevel.Info(l.srcLogger)
	if err := kitlog.With(lWithLevel, msgKey, msg).Log(keyvals...); err != nil {
		errLogger := kitlevel.Error(l.srcLogger)
		kitlog.With(errLogger, msgKey, msg).Log("err", err)
	}
}

// Debug logs a message at level Debug.
func (l *rotateLogger) Debug(msg string, keyvals ...interface{}) {
	lWithLevel := kitlevel.Debug(l.srcLogger)
	if err := kitlog.With(lWithLevel, msgKey, msg).Log(keyvals...); err != nil {
		errLogger := kitlevel.Error(l.srcLogger)
		kitlog.With(errLogger, msgKey, msg).Log("err", err)
	}
}

// Error logs a message at level Error.
func (l *rotateLogger) Error(msg string, keyvals ...interface{}) {
	lWithLevel := kitlevel.Error(l.srcLogger)
	lWithMsg := kitlog.With(lWithLevel, msgKey, msg)
	if err := lWithMsg.Log(keyvals...); err != nil {
		lWithMsg.Log("err", err)
	}
}

// With returns a new contextual logger with keyvals prepended to those passed
// to calls to Info, Debug or Error.
func (l *rotateLogger) With(keyvals ...interface{}) tmlog.Logger {
	return &rotateLogger{kitlog.With(l.srcLogger, keyvals...), l.baseLogger}
}

//------------------------------------------------------------------------------------
// copy from tmfmt_logger.go

type tmfmtEncoder struct {
	*logfmt.Encoder
	buf bytes.Buffer
}

func (l *tmfmtEncoder) Reset() {
	l.Encoder.Reset()
	l.buf.Reset()
}

var tmfmtEncoderPool = sync.Pool{
	New: func() interface{} {
		var enc tmfmtEncoder
		enc.Encoder = logfmt.NewEncoder(&enc.buf)
		return &enc
	},
}

type tmfmtLogger struct {
	w io.Writer
}

// NewTMFmtLogger returns a logger that encodes keyvals to the Writer in
// Tendermint custom format. Note complex types (structs, maps, slices)
// formatted as "%+v".
//
// Each log event produces no more than one call to w.Write.
// The passed Writer must be safe for concurrent use by multiple goroutines if
// the returned Logger will be used concurrently.
func NewTMFmtLogger(w io.Writer) kitlog.Logger {
	return &tmfmtLogger{w}
}

func (l tmfmtLogger) Log(keyvals ...interface{}) error {
	enc := tmfmtEncoderPool.Get().(*tmfmtEncoder)
	enc.Reset()
	defer tmfmtEncoderPool.Put(enc)

	const unknown = "unknown"
	lvl := "none"
	msg := unknown
	module := unknown

	// indexes of keys to skip while encoding later
	excludeIndexes := make([]int, 0)

	for i := 0; i < len(keyvals)-1; i += 2 {
		// Extract level
		if keyvals[i] == kitlevel.Key() {
			excludeIndexes = append(excludeIndexes, i)
			switch keyvals[i+1].(type) {
			case string:
				lvl = keyvals[i+1].(string)
			case kitlevel.Value:
				lvl = keyvals[i+1].(kitlevel.Value).String()
			default:
				panic(fmt.Sprintf("level value of unknown type %T", keyvals[i+1]))
			}
			// and message
		} else if keyvals[i] == msgKey {
			excludeIndexes = append(excludeIndexes, i)
			msg = keyvals[i+1].(string)
			// and module (could be multiple keyvals; if such case last keyvalue wins)
		} else if keyvals[i] == moduleKey {
			excludeIndexes = append(excludeIndexes, i)
			module = keyvals[i+1].(string)
		}
	}

	// Form a custom Tendermint line
	//
	// Example:
	//     D[05-02|11:06:44.322] Stopping AddrBook (ignoring: already stopped)
	//
	// Description:
	//     D										- first character of the level, uppercase (ASCII only)
	//     [05-02|11:06:44.322] - our time format (see https://golang.org/src/time/format.go)
	//     Stopping ...					- message
	enc.buf.WriteString(fmt.Sprintf("%c[%s] %-44s ", lvl[0]-32, time.Now().In(cstZone).Format(timeFormat), msg))

	if module != unknown {
		enc.buf.WriteString("module=" + module + " ")
	}

KeyvalueLoop:
	for i := 0; i < len(keyvals)-1; i += 2 {
		for _, j := range excludeIndexes {
			if i == j {
				continue KeyvalueLoop
			}
		}

		err := enc.EncodeKeyval(keyvals[i], keyvals[i+1])
		if err == logfmt.ErrUnsupportedValueType {
			enc.EncodeKeyval(keyvals[i], fmt.Sprintf("%+v", keyvals[i+1]))
		} else if err != nil {
			return err
		}
	}

	// Add newline to the end of the buffer
	if err := enc.EndRecord(); err != nil {
		return err
	}

	// The Logger interface requires implementations to be safe for concurrent
	// use by multiple goroutines. For this implementation that means making
	// only one call to l.w.Write() for each call to Log.
	if _, err := l.w.Write(enc.buf.Bytes()); err != nil {
		return err
	}
	return nil
}

//------------------------------------------------------------------------------------

// BaseLogger defines the behavior of a log provider.
type BaseLogger interface {
	Init(config []byte, logfile string) error
	Write(msg []byte) (n int, err error)
	Close() error
}

// fileLogWriter implements LoggerInterface.
// It writes messages by lines limit, file size limit, or time frequency.
type fileLogWriter struct {
	inited       uint32
	stoped       uint32
	sync.RWMutex // write log order by order and  atomic incr maxLinesCurLines and maxSizeCurSize
	// The opened file
	Filename   string `json:"filename"`
	fileWriter *os.File

	// Rotate at line
	MaxLines         int `json:"maxlines"`
	maxLinesCurLines int

	// Rotate at size
	MaxSize        int `json:"maxsize"`
	maxSizeCurSize int

	// Rotate daily
	Daily              bool `json:"daily"`
	Hourly             bool `json:"hourly"`
	Minutely           bool `json:"minutely"`
	Minutes            int  `json:"minutes"`
	MaxDays            int  `json:"maxdays"`
	dailyOpenDate      int
	hourlyOpenHour     int
	minutelyOpenMinute int
	dailyOpenTime      time.Time
	timeLayout         string

	buffer     []byte
	flushTimer *time.Timer

	started bool
	Rotate  bool `json:"rotate"`

	symlinkFilename string

	Perm string `json:"perm"`

	RotatePerm string `json:"rotateperm"`

	fileNameOnly, suffix string // like "project.log", project is fileNameOnly and .log is suffix
}

func newFileWriter() *fileLogWriter {
	w := &fileLogWriter{
		inited:     0,
		stoped:     0,
		started:    false,
		Filename:   "tendermint.log",
		Hourly:     true,
		MaxDays:    7,
		Rotate:     true,
		RotatePerm: "0444",
		Perm:       "0664",
	}
	return w
}

// init file logger with json config. config looks like:
//	{
//	"filename":"tendermint.log",
//	"maxLines":10000,
//	"maxsize":1024,
//	"daily":true,
//	"maxDays":15,
//	"rotate":true,
//  "perm":"0600"
//	}
func (w *fileLogWriter) Init(config []byte, logfile string) error {
	if config != nil {
		err := json.Unmarshal(config, w)
		if err != nil {
			return err
		}

	} else if len(logfile) > 0 {
		w.Filename = logfile
	}

	if len(w.Filename) == 0 {
		return errors.New("RotateConfig must have filename")
	}
	w.symlinkFilename = w.Filename
	w.suffix = filepath.Ext(w.Filename)
	w.fileNameOnly = strings.TrimSuffix(w.Filename, w.suffix)
	if w.suffix == "" {
		w.suffix = ".log"
	}

	if w.MaxLines > 0 || w.MaxSize > 0 {
		w.timeLayout = "2006-01-02_150405"
		if w.MaxLines > 0 {
			w.MaxSize = 0
			if w.MaxLines < 100000 {
				w.MaxLines = 100000
			}
		} else if w.MaxSize < 1048576 {
			w.MaxSize = 1048576
		}
		w.Minutely = false
		w.Hourly   = false
		w.Daily    = false
	} else if w.Minutely {
		w.timeLayout = "2006-01-02_1504"
		w.Hourly = false
		w.Daily  = false
		if w.Minutes < 1 || w.Minutes > 30 {
			w.Minutes = 5
		} else {
			for (60 % w.Minutes) != 0 {
				w.Minutes--
			}
		}
	} else if w.Hourly {
		w.timeLayout = "2006-01-02_15"
		w.Daily = false
	} else {
		w.timeLayout = "2006-01-02"
		w.Daily = true
	}

	w.buffer     = make([]byte, 0, bufferSize)
	w.flushTimer = time.NewTimer(flushNever)

	err := w.startLogger()
	if err == nil {
		go w.flush()
	}
	return err
}

func (w *fileLogWriter) flush() {
	for {
		select {
		case <-w.flushTimer.C:
			w.flushTimer.Reset(flushNever)
			w.flushBuffer()
		}
	}
}

func (w *fileLogWriter) flushBuffer() {
	w.Lock()
	if len(w.buffer) > 0 {
		w.fileWriter.Write(w.buffer)
		w.buffer = w.buffer[:0]
	}
	w.Unlock()
}

func (w *fileLogWriter) flushBufferWithLock() {
	w.fileWriter.Write(w.buffer)
	w.buffer = w.buffer[:0]
}

// start file logger. create log file and set to locker-inside file writer.
func (w *fileLogWriter) startLogger() error {
	if atomic.LoadUint32(&w.stoped) == 1 {
		return nil
	}
	file, err := w.createLogFile()
	if err != nil {
		return err
	}
	if w.fileWriter != nil {
		w.fileWriter.Close()
	}
	w.fileWriter = file
	return w.initFd()
}

func (w *fileLogWriter) needRotate(size int, when time.Time) bool {
	return (w.MaxLines > 0 && w.maxLinesCurLines >= w.MaxLines) ||
		(w.MaxSize > 0 && w.maxSizeCurSize >= w.MaxSize) ||
		(w.Hourly && when.Hour() != w.hourlyOpenHour) ||
		(w.Minutely && (when.Minute()-w.minutelyOpenMinute >= w.Minutes || when.Minute() < w.minutelyOpenMinute)) ||
		(w.Daily && when.Day() != w.dailyOpenDate)
}

// Write write logger message into file.
func (w *fileLogWriter) Write(msg []byte) (n int, err error) {
	when := time.Now()
	if w.Rotate {
		w.RLock()
		if w.needRotate(len(msg), when) {
			w.RUnlock()
			w.Lock()
			if w.needRotate(len(msg), when) {
				if err = w.doRotate(when); err != nil {
					fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.Filename, err)
				}
			}
			w.Unlock()
		} else {
			w.RUnlock()
		}
	}

	n = len(msg)
	w.Lock()
	defer w.Unlock()
	prev := len(w.buffer)
	size := len(msg)
	if size+len(w.buffer) <= bufferSize {
		w.buffer = append(w.buffer, msg...)
	} else {
		w.flushBufferWithLock()
		if size > bufferSize/2 {
			_, err = w.fileWriter.Write(msg)
		} else {
			w.buffer = append(w.buffer, msg...)
		}
	}
	if len(w.buffer) == 0 {
		w.flushTimer.Reset(flushNever)
	} else if prev == 0 {
		w.flushTimer.Reset(flushOften)
	}
	w.maxLinesCurLines += 1
	w.maxSizeCurSize   += size

	return
}

func (w *fileLogWriter) createLogFile() (*os.File, error) {
	w.dailyOpenTime = time.Now()
	w.dailyOpenDate = w.dailyOpenTime.Day()
	w.hourlyOpenHour = w.dailyOpenTime.Hour()
	w.minutelyOpenMinute = w.dailyOpenTime.Minute()
	if !w.started && w.Minutes > 1 {
		w.started = true
		diff := w.minutelyOpenMinute % w.Minutes
		w.minutelyOpenMinute -= diff
		w.dailyOpenTime = time.Unix(w.dailyOpenTime.Unix()-int64(diff)*60, 0)
	}
	if w.Rotate {
		if w.MaxLines > 0 || w.MaxSize > 0 {
			// file exists
			// Find the next available number
			num := 1
			var err error

			for ; err == nil && num <= 999; num++ {
				w.Filename = w.fileNameOnly + fmt.Sprintf(".%s-%03d%s", w.dailyOpenTime.Format(w.timeLayout), num, w.suffix)
				_, err = os.Lstat(w.Filename)
			}

			// return error if the last file checked still existed
			if err == nil {
				return nil, fmt.Errorf("Rotate: Cannot find free log number to rename %s", w.Filename)
			}

		} else {
			w.Filename = fmt.Sprintf("%s.%s%s", w.fileNameOnly, w.dailyOpenTime.Format(w.timeLayout), w.suffix)
		}
	}
	// Open the log file
	perm, err := strconv.ParseInt(w.Perm, 8, 64)
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(w.Filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.FileMode(perm))
	if err == nil {
		// Make sure file perm is user set perm cause of `os.OpenFile` will obey umask
		os.Chmod(w.Filename, os.FileMode(perm))
	}
	return fd, err
}

func (w *fileLogWriter) initFd() error {
	fd := w.fileWriter
	fInfo, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("get stat err: %s", err)
	}
	w.maxSizeCurSize = int(fInfo.Size())
	w.maxLinesCurLines = 0
	if w.Rotate {
		os.Remove(w.symlinkFilename)
		err = os.Symlink(filepath.Base(w.Filename), w.symlinkFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Symlink err: %s\n", err)
		}
	}
	if w.Hourly || w.Minutely || w.Daily {
		go w.timelyRotate(w.dailyOpenTime)
	}
	if fInfo.Size() > 0 && w.MaxLines > 0 {
		count, err := w.lines()
		if err != nil {
			return err
		}
		w.maxLinesCurLines = count
	}
	return nil
}

func (w *fileLogWriter) timelyRotate(openTime time.Time) {
	var nextTime time.Time
	if w.Hourly {
		next := openTime.Add(1 * time.Hour)
		y, m, d := next.Date()
		h := next.Hour()
		nextTime = time.Date(y, m, d, h, 0, 0, 0, openTime.Location())
	} else if w.Minutely {
		next := openTime.Add(time.Duration(w.Minutes) * time.Minute)
		y, m, d := next.Date()
		h := next.Hour()
		M := next.Minute()
		nextTime = time.Date(y, m, d, h, M, 0, 0, openTime.Location())
	} else {
		y, m, d := openTime.Add(24 * time.Hour).Date()
		nextTime = time.Date(y, m, d, 0, 0, 0, 0, openTime.Location())
	}
	tm := time.NewTimer(time.Duration(nextTime.UnixNano() - time.Now().UnixNano() + 100))
	<-tm.C
	w.Lock()
	if w.needRotate(0, time.Now()) {
		if err := w.doRotate(time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.Filename, err)
		}
	}
	w.Unlock()
}

func (w *fileLogWriter) lines() (int, error) {
	fd, err := os.Open(w.Filename)
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	buf := make([]byte, 32768) // 32k
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := fd.Read(buf)
		if err != nil && err != io.EOF {
			return count, err
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count, nil
}

// DoRotate means it need to write file in new file.
// new file name like xx.2013-01-01.log (daily) or xx.001.log (by line or size)
func (w *fileLogWriter) doRotate(logTime time.Time) error {
	rotatePerm, err := strconv.ParseInt(w.RotatePerm, 8, 64)
	if err != nil {
		return err
	}

	// close fileWriter before rename
	w.flushTimer.Reset(flushNever)
	w.flushBufferWithLock()
	w.fileWriter.Close()

	err = os.Chmod(w.Filename, os.FileMode(rotatePerm))

	startLoggerErr := w.startLogger()
	go w.deleteOldLog() // TODO: 这里可以控制清理频率

	if startLoggerErr != nil {
		return fmt.Errorf("Rotate StartLogger: %s", startLoggerErr)
	}
	if err != nil {
		return fmt.Errorf("Rotate: %s", err)
	}
	return nil
}

func (w *fileLogWriter) deleteOldLog() {
	dir := filepath.Dir(w.Filename)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) (returnErr error) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "Unable to delete old log '%s', error: %v\n", path, r)
			}
		}()

		if info == nil {
			return
		}

		if !info.IsDir() {
			if info.ModTime().Add(24 * time.Hour * time.Duration(1)).Before(time.Now()) {
				if info.ModTime().Add(24 * time.Hour * time.Duration(w.MaxDays)).Before(time.Now()) {
					if strings.HasPrefix(filepath.Base(path), filepath.Base(w.fileNameOnly)) &&
						strings.HasSuffix(filepath.Base(path), w.suffix) {
						os.Remove(path)
					}
				}
			}
		}
		return
	})
}

// Close close the file description, close file writer.
func (w *fileLogWriter) Close() error {
	if !atomic.CompareAndSwapUint32(&w.stoped, 0, 1) {
		return fmt.Errorf("RotateHandler already stoped")
	}
	w.flushTimer.Reset(flushNever)
	w.flushBuffer()
	w.fileWriter.Sync()
	w.fileWriter.Close()
	return nil
}
