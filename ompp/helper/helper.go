// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

/*
Package helper is a set common helper functions
*/
package helper

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// UnQuote trim spaces and remove "double" or 'single' quotes around string
func UnQuote(src string) string {
	s := strings.TrimSpace(src)
	if len(s) > 1 && (s[0] == '"' || s[0] == '\'') && s[0] == s[len(s)-1] {
		return s[1 : len(s)-1]
	}
	return s
}

// MakeDateTime return date-time string, ie: 2012-08-17 16:04:59.148
func MakeDateTime(t time.Time) string {
	y, mm, dd := t.Date()
	h, mi, s := t.Clock()
	ms := int(time.Duration(t.Nanosecond()) / time.Millisecond)

	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d", y, mm, dd, h, mi, s, ms)
}

// MakeTimeStamp retrun timestamp string as: 2012_08_17_16_04_59_148
func MakeTimeStamp(t time.Time) string {
	y, mm, dd := t.Date()
	h, mi, s := t.Clock()
	ms := int(time.Duration(t.Nanosecond()) / time.Millisecond)

	return fmt.Sprintf("%04d_%02d_%02d_%02d_%02d_%02d_%03d", y, mm, dd, h, mi, s, ms)
}

// CleanPath replace special file path characters: "'`$}{@><:|?*&^;/\ by _ underscore
func CleanPath(src string) string {
	re := regexp.MustCompile("[\"'`$}{@><:|?*&^;/\\\\]")
	return re.ReplaceAllString(src, "_")
}

// ToAlphaNumeric replace all non [A-Z,a-z,0-9] by _ underscore and remove repetitive underscores
func ToAlphaNumeric(src string) string {

	var sb strings.Builder
	isPrevUnder := false

	for _, r := range src {
		if '0' <= r && r <= '9' || 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' {
			sb.WriteRune(r)
			isPrevUnder = false
		} else {
			if isPrevUnder {
				continue // skip repetitive underscore
			}
			sb.WriteRune('_')
			isPrevUnder = true
		}
	}
	return sb.String()
}

// DeepCopy using gob to make a deep copy from src into dst, both src and dst expected to be a pointers
func DeepCopy(src interface{}, dst interface{}) error {
	var bt bytes.Buffer
	enc := gob.NewEncoder(&bt)
	dec := gob.NewDecoder(&bt)

	err := enc.Encode(src)
	if err != nil {
		return errors.New("deep copy encode failed: " + err.Error())
	}

	err = dec.Decode(dst)
	if err != nil {
		return errors.New("deep copy decode failed: " + err.Error())
	}
	return nil
}

// ParseKeyValue string of multiple key=value; pairs separated by semicolon.
// Key cannot be empty, value can be.
// Value can be escaped with "double" or 'single' quotes
func ParseKeyValue(src string) (map[string]string, error) {

	kv := make(map[string]string)
	var key string
	var isKey = true

	for src != "" {

		// split key= and value
		if isKey {
			// skip ; semicolon(s) and spaces in front of the key
			src = strings.TrimLeftFunc(src, func(c rune) bool {
				return c == ';' || unicode.IsSpace(c)
			})
			if src == "" {
				break // empty end of string, no more key=...
			}

			nEq := strings.IndexRune(src, '=')

			if nEq <= 0 {
				return nil, errors.New("expected key=... inside of key=value; string")
			}

			key = strings.TrimSpace(src[:nEq])
			if key == "" {
				return nil, errors.New("invalid (empty) key inside of key=value; string")
			}
			isKey = false
			src = src[nEq+1:] // key is found, skip =
			//continue
		}
		// expected begin of the value position

		// search for end of value ; semicolon, skip quoted part of value
		isQuote := false
		var cQuote rune
		for nPos, chr := range src {

			// if end of value as ; semicolon found
			if !isQuote && chr == ';' {

				// append result to the map, unquote "value" if quotes balanced
				kv[key] = UnQuote(src[:nPos])

				// value is found, skip ; semicolon and reset state
				src = src[nPos+1:]
				key = ""
				isKey = true
				break
			}

			// open or close quotes
			if !isQuote && (chr == '"' || chr == '\'') || isQuote && chr == cQuote {
				isQuote = !isQuote
				if isQuote {
					cQuote = chr // opening quote
				} else {
					cQuote = 0 // quote closed
				}
				continue
			}
		}
		// last key=value without ; semicolon at the end of line
		if !isKey && key != "" {
			kv[key] = UnQuote(src)
			break
		}
	}

	return kv, nil
}
