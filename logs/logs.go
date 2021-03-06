//Copyright 2018 The axx Authors. All rights reserved.

package logs

import (
	"database/sql/driver"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
	"unicode"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger                       *XLogger
	gromDebugLogger              = log.New(os.Stdout, "\r\n", 0)
	gromSQLRegexp                = regexp.MustCompile(`\?`)
	gromNumericPlaceHolderRegexp = regexp.MustCompile(`\$\d+`)
)

//LogConf log config
type LogConf struct {
	OutPath string `json:"outPath"`
	Level   string `json:"level"`
	Debug   bool   `json:"debug"`
}

//XLogger log
type XLogger struct {
	zap.Logger
	logConf *LogConf
}

//GormLogger Gorm log
type GormLogger struct {
}

//Print Gorm日志打印
func (*GormLogger) Print(v ...interface{}) {
	if logger.logConf.Debug {
		msg := gromLogFormatterDebug(v...)
		if msg != nil {
			gromDebugLogger.Println(msg...)
		}
	} else {
		msg, level := gromLogFormatter(v...)
		if msg != nil {
			source, _ := v[1].(string)
			if level == "sql" {
				InfoWithCaller("gorm", source, msg...)
			} else {
				ErrorWithCaller("gorm", source, msg...)
			}
		}
	}
}

//LogInit 初始化日志库
func LogInit(conf *LogConf) *XLogger {
	if logger == nil {
		l := logLevel(conf.Level)
		var w zapcore.WriteSyncer
		var core zapcore.Core
		if !conf.Debug {
			encoderConfig := zap.NewProductionEncoderConfig()
			//encoderConfig.LineEnding = zapcore.DefaultLineEnding
			encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
				enc.AppendString(t.Format("2006-01-02 15:04:05"))
			}
			w = zapcore.AddSync(&lumberjack.Logger{
				Filename:   conf.OutPath,
				MaxSize:    100, // megabytes
				MaxBackups: 3,
				MaxAge:     28, // days
			})
			core = zapcore.NewCore(
				zapcore.NewJSONEncoder(encoderConfig),
				w,
				l,
			)
		} else {
			encoderConfig := zap.NewDevelopmentEncoderConfig()
			encoderConfig.LineEnding = zapcore.DefaultLineEnding
			encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
				enc.AppendString(t.Format("2006-01-02 15:04:05"))
			}

			//jsonDebugging := zapcore.AddSync(ioutil.Discard)
			//jsonErrors := zapcore.AddSync(ioutil.Discard)
			consoleDebugging := zapcore.Lock(os.Stdout)
			consoleErrors := zapcore.Lock(os.Stderr)

			//jsonEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
			consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

			core = zapcore.NewTee(
				//zapcore.NewCore(jsonEncoder, jsonErrors, highPriority),
				zapcore.NewCore(consoleEncoder, consoleErrors, zapcore.FatalLevel),
				//zapcore.NewCore(jsonEncoder, jsonDebugging, lowPriority),
				zapcore.NewCore(consoleEncoder, consoleDebugging, zapcore.DebugLevel),
			)

			// w, _, _ = zap.Open("stdout")
			// core = zapcore.NewCore(
			// 	zapcore.NewConsoleEncoder(encoderConfig),
			// 	w,
			// 	l,
			// )
		}
		logger = &XLogger{Logger: *zap.New(core), logConf: conf}
	}
	return logger
}

//Info info日志记录
func Info(msg string, fields ...zap.Field) {
	InfoWithCaller(msg, "", fields...)
}

//I info日志记录
func I(msg string, fields ...zap.Field) {
	InfoWithCaller(msg, "", fields...)
}

//InfoWithCaller info日志记录
func InfoWithCaller(msg string, caller string, fields ...zap.Field) {
	if logger != nil {
		logger.Info(msg, LogCaller(caller, 0, fields...)...)
	}
}

//Debug debug日志记录
func Debug(msg string, fields ...zap.Field) {
	DebugWithCaller(msg, "", fields...)
}

//D debug日志记录
func D(msg string, fields ...zap.Field) {
	DebugWithCaller(msg, "", fields...)
}

//DebugWithCaller debug日志记录
func DebugWithCaller(msg string, caller string, fields ...zap.Field) {
	if logger != nil {
		logger.Debug(msg, LogCaller(caller, 0, fields...)...)
	}
}

//Error error日志记录
func Error(msg string, fields ...zap.Field) {
	ErrorWithCaller(msg, "", fields...)
}

//E error日志记录
func E(msg string, fields ...zap.Field) {
	ErrorWithCaller(msg, "", fields...)
}

//Err Error日志记录
func Err(msg string, err error) {
	if msg == "" {
		msg = "发生错误"
	}
	if err != nil {
		msg += "，详情：" + err.Error()
	}
	ErrorWithCaller(msg, "")
}

//ErrorWithCaller error日志记录
func ErrorWithCaller(msg string, caller string, fields ...zap.Field) {
	if logger != nil {
		fields = LogCaller(caller, 0, fields...)
		if logger.logConf.Debug {
			fields = append(fields, zap.ByteString("stack", debug.Stack()))
		}
		logger.Error(msg, fields...)
	}
}

//Logger 原始日志对象
func Logger() *XLogger {
	return logger
}

//LoggerGorm Gorm日志对象
func LoggerGorm() *GormLogger {
	return &GormLogger{}
}

//Sync 同步
func Sync() {
	if logger != nil {
		logger.Sync()
	}
}

func logLevel(text string) zapcore.Level {
	switch text {
	case "debug", "DEBUG":
		return zapcore.DebugLevel
	case "info", "INFO", "": // make the zero value useful
		return zapcore.InfoLevel
	case "warn", "WARN":
		return zapcore.WarnLevel
	case "error", "ERROR":
		return zapcore.ErrorLevel
	case "dpanic", "DPANIC":
		return zapcore.DPanicLevel
	case "panic", "PANIC":
		return zapcore.PanicLevel
	case "fatal", "FATAL":
		return zapcore.FatalLevel
	}
	return zapcore.ErrorLevel
}

//LogCaller 获取调用链
func LogCaller(caller string, skip int, fields ...zap.Field) []zap.Field {
	if caller == "" {
		if skip <= 0 {
			skip = 3
		}
		if caller == "" {
			caller = Caller(skip)
		}
	}
	if caller != "" {
		var fs []zap.Field
		if fields != nil {
			fs = fields[:]
		} else {
			fs = make([]zap.Field, 0, 1)
		}
		fs = append(fs, zap.String("caller", caller))
		return fs
	}
	return fields[:]
}

//Caller 获取调用链
func Caller(skip int) string {
	if skip <= 0 {
		skip = 3
	} else {
		skip++
	}
	_, file, line, ok := runtime.Caller(skip)
	if ok {
		caller := file + ":" + strconv.Itoa(line)
		return caller
	}
	return ""
}

/*********************************
* grom
*********************************/

//grom logFormatter
var gromLogFormatterDebug = func(values ...interface{}) (messages []interface{}) {
	if len(values) > 1 {
		var (
			sql             string
			formattedValues []string
			level           = values[0]
			currentTime     = "\n\033[33m[" + time.Now().Format("2006-01-02 15:04:05") + "]\033[0m"
			source          = fmt.Sprintf("\033[35m(%v)\033[0m", values[1])
		)

		messages = []interface{}{source, currentTime}

		if level == "sql" {
			// duration
			messages = append(messages, fmt.Sprintf(" \033[36;1m[%.2fms]\033[0m ", float64(values[2].(time.Duration).Nanoseconds()/1e4)/100.0))

			// sql
			vs := values[4].([]interface{})
			for _, value := range vs {
				indirectValue := reflect.Indirect(reflect.ValueOf(value))
				if indirectValue.IsValid() {
					value = indirectValue.Interface()
					if t, ok := value.(time.Time); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format("2006-01-02 15:04:05")))
					} else if b, ok := value.([]byte); ok {
						if str := string(b); isPrintable(str) {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
						} else {
							formattedValues = append(formattedValues, "'<binary>'")
						}
					} else if r, ok := value.(driver.Valuer); ok {
						if value, err := r.Value(); err == nil && value != nil {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						} else {
							formattedValues = append(formattedValues, "NULL")
						}
					} else {
						switch value.(type) {
						case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
							formattedValues = append(formattedValues, fmt.Sprintf("%v", value))
							break
						default:
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						}
					}
				} else {
					formattedValues = append(formattedValues, "NULL")
				}
			}

			// differentiate between $n placeholders or else treat like ?
			if gromNumericPlaceHolderRegexp.MatchString(values[3].(string)) {
				sql = values[3].(string)
				for index, value := range formattedValues {
					placeholder := fmt.Sprintf(`\$%d([^\d]|$)`, index+1)
					sql = regexp.MustCompile(placeholder).ReplaceAllString(sql, value+"$1")
				}
			} else {
				formattedValuesLength := len(formattedValues)
				vss := gromSQLRegexp.Split(values[3].(string), -1)
				for index, value := range vss {
					sql += value
					if index < formattedValuesLength {
						sql += formattedValues[index]
					}
				}
			}
			messages = append(messages, sql)
			messages = append(messages, fmt.Sprintf(" \n\033[36;31m[%v]\033[0m ", strconv.FormatInt(values[5].(int64), 10)+" rows affected or returned "))

		} else {
			messages = append(messages, "\033[31;1m")
			messages = append(messages, values[2:]...)
			messages = append(messages, "\033[0m")
		}
	}
	return
}

//grom logFormatter
var gromLogFormatter = func(values ...interface{}) (messages []zap.Field, levels string) {
	if len(values) > 1 {
		var (
			sql             string
			formattedValues []string
			level           = values[0]
			currentTime     = ""
			source          = ""
		)
		currentTime = time.Now().Format("2006-01-02 15:04:05")
		source, _ = values[1].(string)

		messages = []zap.Field{}
		messages = append(messages, zap.String("source", source))
		messages = append(messages, zap.String("currentTime", currentTime))

		if level == "sql" {
			levels = "sql"
			timeCost := strconv.FormatFloat(float64(values[2].(time.Duration).Nanoseconds()/1e4)/100.0, 'f', 0, 64) + "ms"
			messages = append(messages, zap.String("timeCost", timeCost))

			// sql
			vs := values[4].([]interface{})
			for _, value := range vs {
				indirectValue := reflect.Indirect(reflect.ValueOf(value))
				if indirectValue.IsValid() {
					value = indirectValue.Interface()
					if t, ok := value.(time.Time); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format("2006-01-02 15:04:05")))
					} else if b, ok := value.([]byte); ok {
						if str := string(b); isPrintable(str) {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
						} else {
							formattedValues = append(formattedValues, "'<binary>'")
						}
					} else if r, ok := value.(driver.Valuer); ok {
						if value, err := r.Value(); err == nil && value != nil {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						} else {
							formattedValues = append(formattedValues, "NULL")
						}
					} else {
						switch value.(type) {
						case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
							formattedValues = append(formattedValues, fmt.Sprintf("%v", value))
							break
						default:
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						}
					}
				} else {
					formattedValues = append(formattedValues, "NULL")
				}
			}
			// differentiate between $n placeholders or else treat like ?
			if gromNumericPlaceHolderRegexp.MatchString(values[3].(string)) {
				sql = values[3].(string)
				for index, value := range formattedValues {
					placeholder := fmt.Sprintf(`\$%d([^\d]|$)`, index+1)
					sql = regexp.MustCompile(placeholder).ReplaceAllString(sql, value+"$1")
				}
			} else {
				formattedValuesLength := len(formattedValues)
				vss := gromSQLRegexp.Split(values[3].(string), -1)
				for index, value := range vss {
					sql += value
					if index < formattedValuesLength {
						sql += formattedValues[index]
					}
				}
			}
			messages = append(messages, zap.String("sql", sql))
			rowsAffected := values[5].(int64)
			messages = append(messages, zap.Int64("rows", rowsAffected))
		} else {
			key, ok := level.(string)
			if !ok {
				key = "log"
			}
			levels = key
			messages = append(messages, zap.Any(key, values[2:]))
		}
	}
	return
}

func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

//ClearLogger 清空日志
func ClearLogger() {
	logger = nil
	gromDebugLogger = nil
	gromLogFormatter = nil
}
