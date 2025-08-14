// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

/*
Package omppLog to println messages to standard output and log file.
It is intended for progress or error logging and should not be used for profiling (it is slow).

Log can be enabled/disabled for two independent streams:

	console  => standard output stream
	log file => log file, truncated on every run, (optional) unique "stamped" name

"Stamped" file name produced by adding time-stamp and/or pid-stamp, i.e.:

	exeName.log => exeName.2012_08_17_16_04_59_148.123.log
*/
package omppLog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/openmpp/go/ompp/config"
	"github.com/openmpp/go/ompp/helper"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// log control state
var (
	theLock       sync.Mutex                           // mutex to lock log control state
	isFileEnabled bool                                 // if true then log to file enabled
	isFileCreated bool                                 // if true then log file created
	logPath       string                               // log file path
	lastYear      int                                  // if daily log then year of current daily stamp
	lastMonth     time.Month                           // if daily log then month current daily stamp
	lastDay       int                                  // if daily log then day current daily stamp
	logOpts       = config.LogOptions{IsConsole: true} // log options, default is log to console
)

// translated messages from message.ini file(s)
var (
	msgLock sync.Mutex                             // mutex to lock for message string search or update
	msgLang string                                 // message language: msgIni section
	msgMap                   = map[string]string{} // message map to find translation
	msgPrt  *message.Printer = nil                 // language specific printer, if not null then use it instead of fmt
)

// return translated string, if translation not found then retrun source string
func LT(src string) string {
	if src == "" {
		return src
	}

	msgLock.Lock()
	defer msgLock.Unlock()

	if t, ok := msgMap[src]; ok {
		return t
	}
	return src
}

// LogIfTime do Log(msg) not more often then every nSeconds.
// It does nothing if time now < lastT + nSeconds. Time is a Unix time, seconds since epoch.
func LogIfTime(lastT int64, nSeconds int64, msg ...any) int64 {

	now := time.Now().Unix()
	if now < lastT+nSeconds {
		return lastT // exit, period is not expired yet
	}
	Log(msg...) // log the message
	return now
}

// New log settings
func New(opts *config.LogOptions) {
	theLock.Lock()
	defer theLock.Unlock()

	if opts != nil {
		logOpts = *opts
	}
	isFileEnabled = logOpts.IsFile // file may be enabled but not created
	isFileCreated = false
}

// Log message to console and log file,
// put space between msg items,
// translate first msg[0] item using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func Log(msg ...any) {
	doLog(false, msg...)
}

// Log message to console and log file,
// put space between msg items
func LogNoLT(msg ...any) {
	doLog(true, msg...)
}

// log formattted message to console and log file,
// translate format string using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func LogFmt(format string, msg ...any) {
	doLogFmt(false, format, msg...)
}

// log formattted message to console and log file,
func LogFmtNoLT(format string, msg ...any) {
	doLogFmt(true, format, msg...)
}

// Log message to console and log file,
// put space between msg items
func doLog(isNoLT bool, msg ...any) {
	now := time.Now()
	m := helper.MakeDateTime(now) + " " + makeMsg(isNoLT, msg...)
	writeToLog(now, m)
}

// return joined msg items by space, use Sprint for each item,
// translate first msg[0] item using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func Msg(msg ...any) string {
	return makeMsg(false, msg...)
}

// return string formatted by Sprintf
// translate format string using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func Fmt(format string, msg ...any) string {

	if format != "" {
		if msgPrt != nil {
			return msgPrt.Sprintf(LT(format), msg...)
		} else {
			return fmt.Sprintf(format, msg...)
		}
	}
	if msgPrt != nil {
		return msgPrt.Sprint(msg...)
	}
	return fmt.Sprint(msg...)
}

// log formattted message to console and log file,
// translate format string using message.ini content
func doLogFmt(isNoLT bool, format string, msg ...any) {

	if format == "" {
		doLog(isNoLT, msg...) // ignore empty format to avoid ugly output
		return
	}

	now := time.Now()
	m := ""

	if !isNoLT && msgPrt != nil {
		m = helper.MakeDateTime(now) + " " + msgPrt.Sprintf(LT(format), msg...)
	} else {
		m = helper.MakeDateTime(now) + " " + fmt.Sprintf(format, msg...)
	}
	writeToLog(now, m)
}

// Join msg items by space and translate first msg[0] item using message.ini content
func makeMsg(isNoLT bool, msg ...any) string {

	if len(msg) <= 0 {
		return ""
	}
	// translate first item of msg[] if it is a string
	m := ""
	if m0, ok := msg[0].(string); ok {
		if !isNoLT {
			m = LT(m0)
		} else {
			m = m0
		}
	} else {
		if !isNoLT && msgPrt != nil {
			m = msgPrt.Sprint(msg[0])
		} else {
			m = fmt.Sprint(msg[0])
		}
	}
	// append the rest of message items
	for _, v := range msg[1:] {
		if !isNoLT && msgPrt != nil {
			m += " " + msgPrt.Sprint(v)
		} else {
			m += " " + fmt.Sprint(v)
		}
	}
	return m
}

// LogSql sql query to file
func LogSql(sql string) {
	theLock.Lock()
	defer theLock.Unlock()

	if !logOpts.IsLogSql { // exit if log sql not enabled
		return
	}

	// create log file if required log to file if file log enabled
	now := time.Now()
	if isFileEnabled &&
		(!isFileCreated ||
			logOpts.IsDaily && (now.Year() != lastYear || now.Month() != lastMonth || now.Day() != lastDay)) {
		isFileCreated = createLogFile(now)
		isFileEnabled = isFileCreated
	}
	if isFileEnabled {
		isFileEnabled = writeToLogFile(helper.MakeDateTime(now) + " " + sql)
	}
}

// create log file or truncate if already exist, return false on errors to disable file log
func createLogFile(nowTime time.Time) bool {

	// make log file path as log settings file path with daily-stamp, if daily stamp required
	logPath = logOpts.LogPath

	if logOpts.IsDaily {
		dir, fName := filepath.Split(logPath)
		ext := filepath.Ext(fName)
		if ext != "" {
			fName = fName[:len(fName)-len(ext)]
		}
		lastYear = nowTime.Year()
		lastMonth = nowTime.Month()
		lastDay = nowTime.Day()
		logPath = filepath.Join(dir, fName+"."+fmt.Sprintf("%04d%02d%02d", lastYear, lastMonth, lastDay)+ext)
	}

	// create log file or truncate existing
	f, err := os.Create(logPath)
	if err != nil {
		return false
	}
	defer f.Close()
	return true
}

// write log message to console and log file
func writeToLog(now time.Time, m string) {
	// log message to console
	theLock.Lock()
	defer theLock.Unlock()

	if logOpts.IsConsole {
		fmt.Println(m)
	}

	// create log file if required log to file if file log enabled
	if isFileEnabled &&
		(!isFileCreated ||
			logOpts.IsDaily && (now.Year() != lastYear || now.Month() != lastMonth || now.Day() != lastDay)) {
		isFileCreated = createLogFile(now)
		isFileEnabled = isFileCreated
	}
	if isFileEnabled {
		isFileEnabled = writeToLogFile(m)
	}
}

// write message to log file, return false on errors to disable file log
func writeToLogFile(msg string) bool {

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = f.WriteString(msg)
	if err == nil {
		if runtime.GOOS == "windows" { // adjust newline for windows
			_, err = f.WriteString("\r\n")
		} else {
			_, err = f.WriteString("\n")
		}
	}
	return err == nil
}

// Load translated messages from language sections of message.ini files.
// It is a merge of exeName.message.ini and go.common.message.ini.
// If exeName is empty or exeName.message.ini file not exist then do nothing
func LoadMessageIni(exeName, exeDir string, userLang string, encodingName string) (string, error) {

	// read message.ini file if exeName is not empty
	if exeName == "" {
		return userLang, nil
	}
	p := filepath.Join(exeDir, exeName+".message.ini")
	if !helper.IsFileExist(p) {
		return userLang, nil // message.ini not found
	}

	msgIni, err := config.ReadMessageIni(exeName, exeDir, encodingName)
	if err != nil {
		return userLang, err // errror at reading or parsing existing message.ini
	}

	// read go-common.message.ini and merge with exe message.ini
	if cmIni, e := config.ReadSharedMessageIni("go-common.message.ini", exeDir, encodingName); e == nil {
		for _, ea := range cmIni {
			msgIni = config.MergeIniEntry(msgIni, ea.Section, ea.Key, ea.Val)
		}
	}

	if len(msgIni) <= 0 {
		return userLang, nil // message.ini is empty
	}

	// match message.ini languages to user language to build message.ini sectioans list ordered by language match confidence
	// known issue: it does not understand .UTF-8
	// examples: en_CA.UTF-8 => en-US, fr_CA.UTF-8 => fr

	mMap := map[string]string{}
	uLang := ""
	uTag := language.Und

	if userLang != "" {
		uTag = language.Make(userLang)
		uLang = uTag.String()
		if uLang == "und" {
			uLang = "" // undefined user language
		}

		scLst := config.IniSectionList(msgIni)
		langLst := make([]string, 0, len(scLst))
		cLst := make([]language.Confidence, 0, len(scLst))

		for _, sn := range scLst {

			lt := []language.Tag{language.Make(sn)}
			_, _, c := language.NewMatcher(lt).Match(uTag)
			if c > language.No {
				langLst = append(langLst, sn)
				cLst = append(cLst, c)
			}
		}

		// sort language list in confidence decsending order
		sort.SliceStable(langLst, func(i, j int) bool { return cLst[i] < cLst[j] })

		// include most confident translation into message map
		for _, ln := range langLst {
			for k := range msgIni {
				if msgIni[k].Section == ln {
					mMap[msgIni[k].Key] = msgIni[k].Val
				}
			}
		}

	}

	// update user language name and translation map
	msgLock.Lock()
	defer msgLock.Unlock()

	if uLang != "" && uLang != "und" {
		msgPrt = message.NewPrinter(uTag)
	}
	msgLang = uLang
	msgMap = mMap

	return msgLang, nil
}
