// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

/*
Package helper is a set common helper functions
*/
package helper

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// translated messages from message.ini file(s)
var (
	msgLock sync.Mutex                             // mutex to lock for message string search or update
	msgLang string                                 // message language: msgIni section
	msgMap                   = map[string]string{} // message map to find translation
	msgPrt  *message.Printer = nil                 // language specific printer, if not null then use it instead of fmt
)

// short form of: errors.New(helper.Msg(msg))
func ErrorMsg(msg ...any) error { return errors.New(Msg(msg...)) }

// short form of: errors.New(helper.Fmt(format, msg))
func ErrorFmt(format string, msg ...any) error { return errors.New(Fmt(format, msg...)) }

// return translated string, if translation not found then retrun source string
func LT(src string) string {
	if src == "" {
		return src
	}
	msgLock.Lock()
	defer msgLock.Unlock()

	return doLT(src)
}

// for internal use only: inside the lock.
// return translated string, if translation not found then retrun source string
func doLT(src string) string {
	if src == "" {
		return src
	}

	if t, ok := msgMap[src]; ok {
		return t
	}
	return src
}

// Join items by space, use Sprint for each item,
// translate first msg[0] item using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func Msg(msg ...any) string {

	if len(msg) <= 0 {
		return ""
	}
	msgLock.Lock()
	defer msgLock.Unlock()

	return doMsg(msg...)
}

// for internal use only: inside the lock.
// Join items by space, use Sprint for each item,
// translate first msg[0] item using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func doMsg(msg ...any) string {

	if len(msg) <= 0 {
		return ""
	}

	// translate first item of msg[] if it is a string
	m := ""
	if m0, ok := msg[0].(string); ok {
		m = doLT(m0)
	} else {
		if msgPrt != nil {
			m = msgPrt.Sprint(msg[0])
		} else {
			m = fmt.Sprint(msg[0])
		}
	}

	// append the rest of message items
	for _, v := range msg[1:] {
		if m != "" {
			m += " "
		}
		if msgPrt != nil {
			m += msgPrt.Sprint(v)
		} else {
			m += fmt.Sprint(v)
		}
	}
	return m
}

// Join msg items by space
// this function should NOT use anything from message store
func MsgNoLT(msg ...any) string {

	// this function should NOT use anything from message store
	// msgLock.Lock()
	// defer msgLock.Unlock()

	m := ""
	for _, v := range msg {
		if m != "" {
			m += " "
		}
		m += fmt.Sprint(v)
	}
	return m
}

// return string formatted by Sprintf
// translate format string using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func Fmt(format string, msg ...any) string {

	msgLock.Lock()
	defer msgLock.Unlock()

	return doFmt(format, msg...)
}

// for internal use only: inside the lock.
// return string formatted by Sprintf
// translate format string using message.ini content,
// use language specific msgPrt printer instead of fmt by default
func doFmt(format string, msg ...any) string {

	if format != "" {
		if msgPrt != nil {
			return msgPrt.Sprintf(doLT(format), msg...)
		} else {
			return fmt.Sprintf(format, msg...)
		}
	}
	if msgPrt != nil {
		return msgPrt.Sprint(msg...)
	}
	return fmt.Sprint(msg...)
}

// update user language name and translation map
func SetMsg(userLang string, msgIni []IniEntry) string {

	// match message.ini languages to user language to build message.ini sectioans list ordered by language match confidence
	// examples: en_CA => en-US, fr_CA => fr
	// known issue: it does not understand .UTF-8 suffix

	mMap := map[string]string{}
	uLang := ""
	uTag := language.Und

	if userLang != "" {
		uTag = language.Make(userLang)
		uLang = uTag.String()
		if uLang == "und" {
			uLang = "" // undefined user language
		}

		scLst := IniSectionList(msgIni)
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

		// sort language list in confidence ascending order
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

	return msgLang
}
