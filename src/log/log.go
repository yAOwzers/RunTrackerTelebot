package log

import (
	"context"
	"flag"
	"fmt"
	"os"

	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

var (
	Logger  zerolog.Logger
	debug   bool
	logpath string
)

func init() {
	flag.BoolVar(&debug, "debug", true, "enable debug log")
	flag.StringVar(&logpath, "logpath", "logs/", "logging path of log file")
}

func InitLogger() {
	// val := flag.Lookup("logpath").Value.(flag.Getter).Get().(string)

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// required to keep nano value
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}
	consoleW := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = "0102 15:04:05.000000"
	})
	fileW := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		// pid := os.Getpid()
		prog := filepath.Base(os.Args[0])
		host, _ := os.Hostname()
		ts := time.Now().Format("20060102150405")
		logfilename := fmt.Sprintf("%s.%s.%s.log", prog, host, ts)
		if err := os.MkdirAll(logpath, os.ModePerm); err != nil {
			fmt.Println("Error: logpath %v", logpath, err)
		}
		logfile := filepath.Join(logpath, logfilename)
		if file, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm); err == nil {
			w.Out = file
		} else {
			fmt.Printf("cannot create log: %v \n", err)
		}
		// file logging can't have color code
		w.NoColor = true
		w.TimeFormat = "0102 15:04:05.000000"
	})
	multi := zerolog.MultiLevelWriter(consoleW, fileW)

	Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
}

func Output(w io.Writer) zerolog.Logger {
	return Logger.Output(w)
}

func With() zerolog.Context {
	return Logger.With()
}

func Sample(s zerolog.Sampler) zerolog.Logger {
	return Logger.Sample(s)
}

func Hook(h zerolog.Hook) zerolog.Logger {
	return Logger.Hook(h)
}

func Err(err error) *zerolog.Event {
	return Logger.Err(err)
}

func Trace() *zerolog.Event {
	return Logger.Trace()
}

func Debug() *zerolog.Event {
	return Logger.Debug()
}

func Info() *zerolog.Event {
	return Logger.Info()
}

func Warn() *zerolog.Event {
	return Logger.Warn()
}

func Error() *zerolog.Event {
	return Logger.Error()
}

func Fatal() *zerolog.Event {
	return Logger.Fatal()
}

func Panic() *zerolog.Event {
	return Logger.Panic()
}

func WithLevel(level zerolog.Level) *zerolog.Event {
	return Logger.WithLevel(level)
}

func Log() *zerolog.Event {
	return Logger.Log()
}

func Print(v ...interface{}) {
	Logger.Print(v...)
}

func Printf(format string, v ...interface{}) {
	Logger.Printf(format, v...)
}

func Ctx(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}
