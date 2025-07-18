// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package ompp

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openmpp/go/ompp/config"
	"github.com/openmpp/go/ompp/db"
)

// read common.message.ini file if exists in one of:
//
//	path/to/exe/common.message.ini
//	OM_ROOT/common.message.ini
//	OM_ROOT/models/common.message.ini
func ReadCommonMessageIni(exeDir string, codePage string) ([]config.IniEntry, error) {

	p := filepath.Join(exeDir, "common.message.ini")
	if cmIni, e := config.NewIni(p, codePage); e == nil && len(cmIni) > 0 {
		return cmIni, e
	}

	if omroot := os.Getenv("OM_ROOT"); omroot != "" {
		p := filepath.Join(omroot, "common.message.ini")
		if cmIni, e := config.NewIni(p, codePage); e == nil && len(cmIni) > 0 {
			return cmIni, e
		}

		p = filepath.Join(omroot, "models", "common.message.ini")
		return config.NewIni(p, codePage)
	}
	return []config.IniEntry{}, nil
}

// read path/to/exe/modelName.message.ini file if exists
func ReadModelMessageIni(modelName, exeDir string, codePage string) ([]config.IniEntry, error) {

	p := filepath.Join(exeDir, modelName+".message.ini")
	return config.NewIni(p, codePage)
}

// create model translated strings list from lang_word and model_word
func NewLangMsg(lwLst []db.LangWord, mwLst []db.ModelLangWord) []db.LangMsg {

	msgLst := []db.LangMsg{}

	// copy lang_word translated strings
	for _, lw := range lwLst {
		msgLst = append(msgLst,
			db.LangMsg{
				LangCode: lw.LangCode,
				Msg:      maps.Clone(lw.Words),
			})
	}

	// insert or update model_word rows into translated strings
	for _, mw := range mwLst {
		// check model_word language: skip if not exists in lang_lst, it may be deleted from model_word
		if j := slices.IndexFunc(msgLst, func(lm db.LangMsg) bool { return lm.LangCode == mw.LangCode }); j >= 0 {
			maps.Copy(msgLst[j].Msg, mw.Words)
		}
	}
	return msgLst
}

// merge of model.message.ini, common.message.ini,
// use content of message.ini to append languages and insert or update translated strings
func AppendLangMsgFromIni(msgLst []db.LangMsg, eiLst []config.IniEntry) []db.LangMsg {

	for _, ei := range eiLst {

		n := slices.IndexFunc(msgLst, func(lm db.LangMsg) bool { return strings.EqualFold(lm.LangCode, ei.Section) })

		if n < 0 { // append new language from ini file

			n = len(msgLst)
			msgLst = append(msgLst,
				db.LangMsg{
					LangCode: ei.Section,
					Msg:      map[string]string{},
				})
		}
		msgLst[n].Msg[ei.Key] = ei.Val // insrert or replace transalted string from ini file
	}
	return msgLst
}
