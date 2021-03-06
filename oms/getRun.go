// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/omppLog"
	"golang.org/x/text/language"
)

// RunStatus return run_lst db row and run_progress db rows by model digest-or-name and run digest-or-stamp-or-name.
func (mc *ModelCatalog) RunStatus(dn, rdsn string) (*db.RunPub, bool) {

	// if model digest-or-name or run digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.RunPub{}, false
	}
	if rdsn == "" {
		omppLog.Log("Warning: invalid (empty) run digest or stamp or name")
		return &db.RunPub{}, false
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// get run_lst db row by digest, stamp or run name
	r, err := db.GetRunByDigestOrStampOrName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, rdsn)
	if err != nil {
		omppLog.Log("Error at get run status: ", dn, ": ", rdsn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}
	if r == nil {
		// omppLog.Log("Warning run status not found: ", dn, ": ", rdsn)
		return &db.RunPub{}, false // return empty result: run_lst row not found
	}

	// get run sub-values progress for that run id
	rpRs, err := db.GetRunProgress(mc.modelLst[idx].dbConn, r.RunId)
	if err != nil {
		omppLog.Log("Error at get run progress: ", dn, ": ", rdsn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run progress select error
	}

	// convert to "public" format
	rp, err := (&db.RunMeta{Run: *r, Progress: rpRs}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at run status conversion: ", dn, ": ", rdsn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result
	}

	return rp, true
}

// RunStatusList return list of run_lst rows joined to run_progress by model digest-or-name and run digest-or-stamp-or-name.
func (mc *ModelCatalog) RunStatusList(dn, rdsn string) ([]db.RunPub, bool) {

	// if model digest-or-name or run digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.RunPub{}, false
	}
	if rdsn == "" {
		omppLog.Log("Warning: invalid (empty) run digest or stamp or name")
		return []db.RunPub{}, false
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.RunPub{}, false // return empty result: model not found or error
	}

	// get run_lst db row by digest, stamp or run name
	rLst, err := db.GetRunListByDigestOrStampOrName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, rdsn)
	if err != nil {
		omppLog.Log("Error at get run status: ", dn, ": ", rdsn, ": ", err.Error())
		return []db.RunPub{}, false // return empty result: run select error
	}
	if len(rLst) <= 0 {
		// omppLog.Log("Warning run status not found: ", dn, ": ", rdsn)
		return []db.RunPub{}, false // return empty result: run_lst row not found
	}

	// for each run_lst row join run_progress rows
	rpLst := []db.RunPub{}

	for n := range rLst {
		// get run sub-values progress for that run id
		rpRs, err := db.GetRunProgress(mc.modelLst[idx].dbConn, rLst[n].RunId)
		if err != nil {
			omppLog.Log("Error at get run progress: ", dn, ": ", rdsn, ": ", err.Error())
			return []db.RunPub{}, false // return empty result: run progress select error
		}

		// convert to "public" format
		rp, err := (&db.RunMeta{Run: rLst[n], Progress: rpRs}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		if err != nil {
			omppLog.Log("Error at run status conversion: ", dn, ": ", rdsn, ": ", err.Error())
			return []db.RunPub{}, false // return empty result
		}
		rpLst = append(rpLst, *rp)
	}
	return rpLst, true
}

// FirstOrLastRunStatus return first or last or last completed run_lst db row and run_progress db rows by model digest-or-name.
func (mc *ModelCatalog) FirstOrLastRunStatus(dn string, isFisrt, isCompleted bool) (*db.RunPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.RunPub{}, false
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// get first or last or last completed run_lst db row
	rst := &db.RunRow{}
	var err error

	if isFisrt {
		rst, err = db.GetFirstRun(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId)
	} else {
		if !isCompleted {
			rst, err = db.GetLastRun(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId)
		} else {
			rst, err = db.GetLastCompletedRun(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId)
		}
	}
	if err != nil {
		omppLog.Log("Error at get run status: ", dn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}
	if rst == nil {
		// omppLog.Log("Warning: there is no run status not found for the model: ", dn)
		return &db.RunPub{}, false // return empty result: run_lst row not found
	}

	// get run sub-values progress for that run id
	rpRs, err := db.GetRunProgress(mc.modelLst[idx].dbConn, rst.RunId)
	if err != nil {
		omppLog.Log("Error at get run progress: ", dn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run progress select error
	}

	// convert to "public" format
	rp, err := (&db.RunMeta{Run: *rst, Progress: rpRs}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at run status conversion: ", dn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result
	}

	return rp, true
}

// RunRowList return list of run_lst db rows by model digest and run digest-stamp-or-name sorted by run_id.
// If there are multiple rows with same run stamp or run digest then multiple rows returned.
func (mc *ModelCatalog) RunRowList(digest string, rdsn string) ([]db.RunRow, bool) {

	// if model digest is empty then return empty results
	if digest == "" {
		omppLog.Log("Warning: invalid (empty) model digest")
		return []db.RunRow{}, false
	}

	// lock catalog and find model index by digest
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigest(digest)
	if !ok {
		omppLog.Log("Warning: model digest not found: ", digest)
		return []db.RunRow{}, false
	}

	// get run_lst db rows by digest, stamp or run name
	rLst, err := db.GetRunListByDigestOrStampOrName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, rdsn)
	if err != nil {
		omppLog.Log("Error at get run status: ", digest, ": ", rdsn, ": ", err.Error())
		return []db.RunRow{}, false // return empty result: run select error
	}
	return rLst, true
}

// RunRowAfterList return list of run_lst db rows by model digest, sorted by run_id.
// If afterRunId > 0 then return only runs where run_id > afterRunId.
func (mc *ModelCatalog) RunRowAfterList(digest string, afterRunId int) ([]db.RunRow, bool) {

	// if model digest is empty then return empty results
	if digest == "" {
		omppLog.Log("Warning: invalid (empty) model digest")
		return []db.RunRow{}, false
	}

	// lock catalog and find model index by digest
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigest(digest)
	if !ok {
		omppLog.Log("Warning: model digest not found: ", digest)
		return []db.RunRow{}, false
	}

	// get run list
	rl, err := db.GetRunList(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, afterRunId)
	if err != nil {
		omppLog.Log("Error at get run list: ", digest, ": ", err.Error())
		return []db.RunRow{}, false // return empty result: run select error
	}
	return rl, true
}

// RunList return list of run_lst db rows by model digest-or-name.
// No text info returned (no description and notes).
func (mc *ModelCatalog) RunList(dn string) ([]db.RunPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.RunPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.RunPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.RunPub{}, false // return empty result: model not found or error
	}

	rl, err := db.GetRunList(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, 0)
	if err != nil {
		omppLog.Log("Error at get run list: ", dn, ": ", err.Error())
		return []db.RunPub{}, false // return empty result: run select error
	}
	if len(rl) <= 0 {
		return []db.RunPub{}, true // return empty result: run_lst rows not found for that model
	}

	// for each run_lst convert it to "public" run format
	rpl := make([]db.RunPub, len(rl))

	for ni := range rl {

		p, err := (&db.RunMeta{Run: rl[ni]}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		if err != nil {
			omppLog.Log("Error at run conversion: ", dn, ": ", err.Error())
			return []db.RunPub{}, false // return empty result: conversion error
		}
		if p != nil {
			rpl[ni] = *p
		}
	}

	return rpl, true
}

// RunListText return list of run_lst and run_txt db rows by model digest-or-name.
// Text (description and notes) are in preferred language or if text in such language exists.
func (mc *ModelCatalog) RunListText(dn string, preferredLang []language.Tag) ([]db.RunPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.RunPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.RunPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.RunPub{}, false // return empty result: model not found or error
	}

	// get run_txt db row for each run_lst using matched preferred language
	_, np, _ := mc.modelLst[idx].matcher.Match(preferredLang...)
	lc := mc.modelLst[idx].langCodes[np]

	rl, rt, err := db.GetRunListText(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, lc)
	if err != nil {
		omppLog.Log("Error at get run list: ", dn, ": ", err.Error())
		return []db.RunPub{}, false // return empty result: run select error
	}
	if len(rl) <= 0 {
		return []db.RunPub{}, true // return empty result: run_lst rows not found for that model
	}

	// for each run_lst find run_txt row if exist and convert to "public" run format
	rpl := make([]db.RunPub, len(rl))

	nt := 0
	for ni := range rl {

		// find text row for current master row by run id
		isFound := false
		for ; nt < len(rt); nt++ {
			isFound = rt[nt].RunId == rl[ni].RunId
			if rt[nt].RunId >= rl[ni].RunId {
				break // text found or text missing: text run id ahead of master run id
			}
		}

		// convert to "public" format
		var p *db.RunPub
		var err error

		if isFound && nt < len(rt) {
			p, err = (&db.RunMeta{Run: rl[ni], Txt: []db.RunTxtRow{rt[nt]}}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		} else {
			p, err = (&db.RunMeta{Run: rl[ni]}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		}
		if err != nil {
			omppLog.Log("Error at run conversion: ", dn, ": ", err.Error())
			return []db.RunPub{}, false // return empty result: conversion error
		}
		if p != nil {
			rpl[ni] = *p
		}
	}

	return rpl, true
}

// RunFull return full run metadata (without text) by model digest-or-name and run digest-or-stamp-or-name.
// It does not return run text metadata: decription and notes from run_txt and run_parameter_txt tables.
func (mc *ModelCatalog) RunFull(dn, rdsn string) (*db.RunPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.RunPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// get run_lst db row by digest, stamp or run name
	r, err := db.GetRunByDigestOrStampOrName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, rdsn)
	if err != nil {
		omppLog.Log("Error at get run status: ", dn, ": ", rdsn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}
	if r == nil {
		omppLog.Log("Warning run status not found: ", dn, ": ", rdsn)
		return &db.RunPub{}, false // return empty result: run_lst row not found
	}

	// get full metadata db rows
	rm, err := db.GetRunFull(mc.modelLst[idx].dbConn, r)
	if err != nil {
		omppLog.Log("Error at get run: ", dn, ": ", r.Name, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}

	// convert to "public" model run format
	rp, err := rm.ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at completed run conversion: ", dn, ": ", r.Name, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: conversion error
	}

	return rp, true
}

// RunTextFull return full run metadata (including text) by model digest-or-name and run digest-or-stamp-or-name.
// It does not return non-completed runs (run in progress).
// Run completed if run status one of: s=success, x=exit, e=error.
// Text (description and notes) can be in preferred language or all languages.
// If preferred language requested and it is not found in db then return empty text results.
func (mc *ModelCatalog) RunTextFull(dn, rdsn string, isAllLang bool, preferredLang []language.Tag) (*db.RunPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.RunPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.RunPub{}, false // return empty result: model not found or error
	}

	// get run_lst db row by digest, stamp or run name
	r, err := db.GetRunByDigestOrStampOrName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, rdsn)
	if err != nil {
		omppLog.Log("Error at get run status: ", dn, ": ", rdsn, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}
	if r == nil {
		omppLog.Log("Warning run status not found: ", dn, ": ", rdsn)
		return &db.RunPub{}, false // return empty result: run_lst row not found
	}

	// get full metadata db rows using matched preferred language or in all languages
	lc := ""
	if !isAllLang {
		_, np, _ := mc.modelLst[idx].matcher.Match(preferredLang...)
		lc = mc.modelLst[idx].langCodes[np]
	}

	rm, err := db.GetRunFullText(mc.modelLst[idx].dbConn, r, lc)
	if err != nil {
		omppLog.Log("Error at get run text: ", dn, ": ", r.Name, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: run select error
	}

	// convert to "public" model run format
	rp, err := rm.ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at completed run conversion: ", dn, ": ", r.Name, ": ", err.Error())
		return &db.RunPub{}, false // return empty result: conversion error
	}

	return rp, true
}
