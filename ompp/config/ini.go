// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/openmpp/go/ompp/helper"
)

// Ini-file entry: section, key and value
type IniEntry struct {
	Section string // section: in lower case, without [], space trimed
	Key     string // key: in lower case, without =, space trimed
	Val     string // value: space trimed and unquoted ("quotes" or 'apostrophes' removed)
}

/*
NewIni read ini-file content into  IniEntry[] of { Section, Key Value }

It is very light and able to parse:

	dsn = "DSN='server'; UID='user'; PWD='pas#word';"   ; comments are # here

Section and key are trimmed and cannot contain comments ; or # chars inside.
Key and values trimmed and "unquoted".
Key or value escaped with "double" or 'single' quotes can include spaces or ; or # chars

Example:

	; comments can start from ; or
	# from # and empty lines are skipped

	[section]  ; section comment
	val = no comment
	rem = ; comment only and empty value
	nul =
	dsn = "DSN='server'; UID='user'; PWD='pas#word';" ; quoted value
	t w = the "# quick #" brown 'fox ; jumps' over    ; escaped: ; and # chars
	" key "" 'quoted' here " = some value

	[multi-line]
	trim = Aname, \    ; multi-line value joined with spaces trimmed
	Bname, \    ; result is:
	CName       ; Aname,Bname,Cname

	; multi-line value started with " quote or ' apostrophe
	; right spaces before \ is not trimmed
	; result is:
	; Multi line   text with spaces
	;
	keep = "\
	       Multi line   \
	       text with spaces\
	       "
*/
func NewIni(iniPath string, encodingName string) ([]IniEntry, error) {

	if iniPath == "" {
		return []IniEntry{}, nil // no ini-file
	}

	// read ini-file and convert to utf-8
	s, err := helper.FileToUtf8(iniPath, encodingName)
	if err != nil {
		return []IniEntry{}, errors.New("reading ini-file to utf-8 failed: " + err.Error())
	}

	// parse ini-file into strings map of (section.key)=>value
	eaIni, err := loadIni(s)
	if err != nil {
		return []IniEntry{}, errors.New("reading ini-file failed: " + err.Error())
	}

	return eaIni, nil
}

// Parse ini-file content into strings map of (section.key)=>value
func loadIni(iniContent string) ([]IniEntry, error) {

	eaIni := []IniEntry{}

	var section, key, val, line string
	var isContinue, isQuote bool
	var cQuote rune

	for nLine, nStart := 0, 0; nStart < len(iniContent); {

		// get current line and move to next
		nextPos := strings.IndexAny(iniContent[nStart:], "\r\n")
		if nextPos < 0 {
			nextPos = len(iniContent)
		}
		if nStart+nextPos < len(iniContent)-1 {
			if iniContent[nStart+nextPos] == '\r' && iniContent[nStart+nextPos+1] == '\n' {
				nextPos++
			}
		}
		nextPos += 1 + nStart
		if nextPos > len(iniContent) {
			nextPos = len(iniContent)
		}

		line = strings.TrimSpace(iniContent[nStart:nextPos])
		nStart = nextPos
		nLine++

		// skip empty line and if it is end of continuation \ value then push it into ini content
		if len(line) <= 0 {

			if key != "" {
				eaIni = MergeIniEntry(eaIni, section, key, helper.UnQuote(val))

				key, val, isContinue, isQuote, cQuote = "", "", false, false, 0 // reset state
			}
			continue // skip empty line
		}

		// remove ; comments or # Linux comments:
		//   comment starts with ; or # outside of "quote" or 'single quote'
		// get the key:
		//   find first = outside of "quote" or 'single quote'
		// get value:
		//    value can be after key= or entire line if it is a continuation \ value
		nEq := len(line) + 1
		nRem := len(line) + 1

		for k, c := range line {

			if !isQuote && (c == '"' || c == '\'') || isQuote && c == cQuote { // open or close quotes
				isQuote = !isQuote
				if isQuote {
					cQuote = c // opening quote
				} else {
					cQuote = 0 // quote closed
				}
				continue
			}
			if !isQuote && c == '=' && (nEq < 0 || nEq >= len(line)) { // positions of first key= outside of quote
				nEq = k
			}
			if !isQuote && (c == ';' || c == '#') { // start of comment outside of quotes
				nRem = k
				break
			}
		}

		// remove comment from the end of the line
		if nRem < len(line) {
			line = strings.TrimSpace(line[:nRem])
		}
		// skip line: it is a comment only line
		// if it is end of continuation \ value then push it into ini content
		if len(line) <= 0 {

			if key != "" {
				eaIni = MergeIniEntry(eaIni, section, key, helper.UnQuote(val))
				key, val, isContinue, isQuote, cQuote = "", "", false, false, 0 // reset state
			}
			continue // skip line: it is a comment only line
		}

		// check for the [section]
		// section is not allowed after previous line \ continuation
		// section cannot have empty [] name
		if !isQuote {

			if line[0] == '[' {

				if len(line) < 3 || line[len(line)-1] != ']' {
					return nil, errors.New("line " + strconv.Itoa(nLine) + ": " + "invalid section name")
				}
				if isContinue {
					return nil, errors.New("line " + strconv.Itoa(nLine) + ": " + "invalid section name as line continuation")
				}

				section = strings.TrimSpace(line[1 : len(line)-1]) // [section] found
				continue
			}
		}
		if section == "" {
			return nil, errors.New("line " + strconv.Itoa(nLine) + ": " + "only comments or empty lines can be before first section")
		}

		// line is not empty: it must start with key= or it can be continuation of previous line value
		if key == "" {

			if nEq < 1 || nEq >= len(line) {
				return nil, errors.New("line " + strconv.Itoa(nLine) + ": " + "expected key=value")
			}
			key = helper.UnQuote(line[:nEq])
			line = strings.TrimSpace(line[nEq+1:])
		}
		if key == "" {
			return nil, errors.New("line " + strconv.Itoa(nLine) + ": " + "expected key=value")
		}

		// if line end with continuation \ then append line to value
		// else push key and value into ini content
		isContinue = len(line) > 0 && line[len(line)-1] == '\\'

		if isContinue {
			if isQuote {
				val = val + strings.TrimLeftFunc(line[:len(line)-1], unicode.IsSpace)
			} else {
				val = val + strings.TrimSpace(line[:len(line)-1])
			}
		} else {

			val = val + line
			eaIni = MergeIniEntry(eaIni, section, key, helper.UnQuote(val))
			key, val, isContinue, isQuote, cQuote = "", "", false, false, 0 // reset state
		}
	}

	// last line: continuation at last line without cr-lf
	if isContinue && section != "" && key != "" {
		eaIni = MergeIniEntry(eaIni, section, key, helper.UnQuote(val))
	}

	return eaIni, nil
}

// Insert new or update existing ini file entry:
// search by (section, key) in source []IniEntry
// if not found then insert new entry
// if found and isUpdate true then update existing entry with new value
func AddIniEntry(isUpdate bool, eaIni []IniEntry, section, key, val string) []IniEntry {

	scIdx := -1
	for k := 0; scIdx < 0 && k < len(eaIni); k++ {
		if strings.EqualFold(eaIni[k].Section, section) {
			scIdx = k
		}
	}
	if scIdx < 0 { // section not found: append new section, key value

		return append(eaIni, IniEntry{Section: section, Key: key, Val: val})

	} // else section found: search key inside of the section

	for ; scIdx < len(eaIni) && strings.EqualFold(eaIni[scIdx].Section, section); scIdx++ {
		if strings.EqualFold(eaIni[scIdx].Key, key) {
			if isUpdate {
				eaIni[scIdx].Val = val // section and key found: update existing section.key with new value
			}
			return eaIni // key found: return []IniEnty array with updated value or original value
		}
	}
	// else key not found in the section: insert new key at the end of section

	return slices.Insert(eaIni, scIdx, IniEntry{Section: section, Key: key, Val: val})
}

// Update existing or insert new ini file entry:
// search by (section, key) in source []IniEntry
// if not found then insert new entry
// if found then update existing entry with new value
func MergeIniEntry(eaIni []IniEntry, section, key, val string) []IniEntry {
	return AddIniEntry(true, eaIni, section, key, val)
}

// Insert new ini file entry if not already exists:
// search by (section, key) in source []IniEntry
// if not found then insert new entry
func InsertIniEntry(eaIni []IniEntry, section, key, val string) []IniEntry {
	return AddIniEntry(false, eaIni, section, key, val)
}

// Update existing or insert new ini file entry:
// sectionKey expected to be: section.key if not then exit and do nothing.
// find section and key in source []IniEntry
// if not found then insert new entry
// if found then update existing entry with new value
func MergeSectionKeyIniEntry(eaIni []IniEntry, sectionKey, val string) []IniEntry {

	sck := strings.SplitN(sectionKey, ".", 2) // expected section.key
	if len(sck) < 2 || sck[0] == "" || sck[1] == "" {
		return eaIni // skip invalid section.key
	}
	return MergeIniEntry(eaIni, sck[0], sck[1], val)
}

// return ini-file sections
func IniSectionList(eaIni []IniEntry) []string {

	scLst := []string{}
	sc := ""

	for _, e := range eaIni {
		if e.Section != sc {
			scLst = append(scLst, e.Section)
		}
		sc = e.Section
	}
	return scLst
}

// read common.message.ini file if exists in one of:
//
//	path/to/exe/common.message.ini
//	OM_ROOT/common.message.ini
//	OM_ROOT/models/common.message.ini
func ReadCommonMessageIni(exeDir string, encodingName string) ([]IniEntry, error) {
	return ReadSharedMessageIni("common.message.ini", exeDir, encodingName)
}

// read commonName.message.ini file if exists in one of:
//
//	path/to/exe/commonName.message.ini
//	OM_ROOT/commonName.message.ini
//	OM_ROOT/models/commonName.message.ini
//
// if commonName is empty then return empty result
func ReadSharedMessageIni(sharedName, exeDir string, encodingName string) ([]IniEntry, error) {

	if sharedName == "" {
		return []IniEntry{}, nil
	}
	if cmIni, e := ReadMessageIni(sharedName, exeDir, encodingName); e == nil && len(cmIni) > 0 {
		return cmIni, e
	}
	if omroot := os.Getenv("OM_ROOT"); omroot != "" {
		if cmIni, e := ReadMessageIni(sharedName, omroot, encodingName); e == nil && len(cmIni) > 0 {
			return cmIni, e
		}
		return ReadMessageIni(sharedName, filepath.Join(omroot, "models"), encodingName)
	}
	return []IniEntry{}, nil
}

// read path/to/exe/name.message.ini,
// it does not return error if message.ini not exist
func ReadMessageIni(name, dir string, encodingName string) ([]IniEntry, error) {

	if name == "" {
		return []IniEntry{}, nil // file name is empty
	}
	p := filepath.Join(dir, name+".message.ini")

	if !helper.IsFileExist(p) {
		return []IniEntry{}, nil // message.ini not found
	}

	return NewIni(p, encodingName)
}
