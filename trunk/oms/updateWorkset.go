// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"container/list"
	"encoding/csv"
	"errors"
	"io"

	"go.openmpp.org/ompp/db"
	"go.openmpp.org/ompp/omppLog"
)

// UpdateWorksetReadonly update workset read-only status by model digest-or-name and workset name.
func (mc *ModelCatalog) UpdateWorksetReadonly(dn, wsn string, isReadonly bool) (string, *db.WorksetRow, bool) {

	// if model digest-or-name or workset name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return "", &db.WorksetRow{}, false
	}
	if wsn == "" {
		omppLog.Log("Warning: invalid (empty) workset name")
		return "", &db.WorksetRow{}, false
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return "", &db.WorksetRow{}, false // return empty result: model not found or error
	}

	// find workset in database
	w, err := db.GetWorksetByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, wsn)
	if err != nil {
		omppLog.Log("Error at get workset status: ", dn, ": ", wsn, ": ", err.Error())
		return "", &db.WorksetRow{}, false // return empty result: workset select error
	}
	if w == nil {
		omppLog.Log("Warning workset status not found: ", dn, ": ", wsn)
		return "", &db.WorksetRow{}, false // return empty result: workset_lst row not found
	}

	// update workset readonly status
	err = db.UpdateWorksetReadonly(mc.modelLst[idx].dbConn, w.SetId, isReadonly)
	if err != nil {
		omppLog.Log("Error at update workset status: ", dn, ": ", wsn, ": ", err.Error())
		return "", &db.WorksetRow{}, false // return empty result: workset select error
	}

	// get workset status
	w, err = db.GetWorkset(mc.modelLst[idx].dbConn, w.SetId)
	if err != nil {
		omppLog.Log("Error at get workset status: ", dn, ": ", w.SetId, ": ", err.Error())
		return "", &db.WorksetRow{}, false // return empty result: workset select error
	}
	if w == nil {
		omppLog.Log("Warning workset status not found: ", dn, ": ", wsn)
		return "", &db.WorksetRow{}, false // return empty result: workset_lst row not found
	}

	return mc.modelLst[idx].meta.Model.Digest, w, true
}

// UpdateWorkset update workset metadata: create new workset, replace existsing or merge metadata.
func (mc *ModelCatalog) UpdateWorkset(isReplace bool, wp *db.WorksetPub) (bool, bool, error) {

	// if model digest-or-name or workset name is empty then return empty results
	dn := wp.ModelDigest
	if dn == "" {
		dn = wp.ModelName
	}
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, false, nil
	}
	if wp.Name == "" {
		omppLog.Log("Warning: invalid (empty) workset name")
		return false, false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and update workset
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	// find workset in database: it must be read-write if exists
	w, err := db.GetWorksetByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, wp.Name)
	if err != nil {
		omppLog.Log("Error at get workset status: ", dn, ": ", wp.Name, ": ", err.Error())
		return false, false, err
	}
	if w != nil && w.IsReadonly {
		omppLog.Log("Failed to update read-only workset: ", dn, ": ", wp.Name)
		return false, false, errors.New("Failed to update read-only workset: " + dn + ": " + wp.Name)
	}

	// if workset does not exist then clean paramters list to create empty workset
	isEraseParam := w == nil && len(wp.Param) > 0
	if isEraseParam {
		wp.Param = []db.ParamRunSetPub{}
		omppLog.Log("Warning: workset not found, create new empty workset (without parameters): ", wp.Name)
	}

	// convert workset from "public" into db rows
	wm, err := wp.FromPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at workset json conversion: ", dn, ": ", wp.Name, ": ", err.Error())
		return false, isEraseParam, err
	}

	// update workset metadata
	err = wm.UpdateWorkset(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta, isReplace, mc.modelLst[idx].langMeta)
	if err != nil {
		omppLog.Log("Error at update workset: ", dn, ": ", wp.Name, ": ", err.Error())
		return false, isEraseParam, err
	}

	return true, isEraseParam, nil
}

// DeleteWorkset do delete workset, including parameter values from database.
func (mc *ModelCatalog) DeleteWorkset(dn, wsn string) (bool, error) {

	// if model digest-or-name or workset name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if wsn == "" {
		omppLog.Log("Warning: invalid (empty) workset name")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return false, nil // return empty result: model not found or error
	}

	// lock catalog and update workset
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	// find workset in database
	w, err := db.GetWorksetByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, wsn)
	if err != nil {
		omppLog.Log("Error at get workset status: ", dn, ": ", wsn, ": ", err.Error())
		return false, err
	}
	if w == nil {
		return false, nil // return OK: workset not found
	}

	// delete workset from database
	err = db.DeleteWorkset(mc.modelLst[idx].dbConn, w.SetId)
	if err != nil {
		omppLog.Log("Error at update workset: ", dn, ": ", wsn, ": ", err.Error())
		return false, err
	}

	return true, nil
}

// UpdateWorksetParameter replace or merge parameter metadata into workset and replace parameter values from csv reader.
func (mc *ModelCatalog) UpdateWorksetParameter(
	isReplace bool, wp *db.WorksetPub, param *db.ParamRunSetPub, csvRd *csv.Reader) (bool, error) {

	// if model digest-or-name or workset name is empty then return empty results
	dn := wp.ModelDigest
	if dn == "" {
		dn = wp.ModelName
	}
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if wp.Name == "" {
		omppLog.Log("Warning: invalid (empty) workset name")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and update workset
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	// convert workset from "public" into db rows
	wm, err := wp.FromPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at workset json conversion: ", dn, ": ", wp.Name, ": ", err.Error())
		return false, err
	}

	// if csv file exist then read csv file and convert and append lines into cell list
	cLst := list.New()

	if csvRd != nil {

		// converter from csv row []string to db cell
		var cell db.CellParam
		cvt, err := cell.CsvToCell(mc.modelLst[idx].meta, param.Name, param.SubCount, "")
		if err != nil {
			return false, errors.New("invalid converter from csv row: " + err.Error())
		}

		isFirst := true
	ReadFor:
		for {
			row, err := csvRd.Read()
			switch {
			case err == io.EOF:
				break ReadFor
			case err != nil:
				return false, errors.New("Failed to read csv parameter values " + param.Name)
			}

			// skip header line
			if isFirst {
				isFirst = false
				continue
			}

			// convert and append cell to cell list
			c, err := cvt(row)
			if err != nil {
				return false, errors.New("Failed to convert csv parameter values " + param.Name)
			}
			cLst.PushBack(c)
		}
		if cLst.Len() <= 0 {
			return false, errors.New("workset: " + wp.Name + " parameter empty: " + param.Name)
		}
	}

	// update workset parameter metadata and parameter values
	hId, err := wm.UpdateWorksetParameter(
		mc.modelLst[idx].dbConn, mc.modelLst[idx].meta, isReplace, param, cLst, mc.modelLst[idx].langMeta)
	if err != nil {
		omppLog.Log("Error at update workset: ", dn, ": ", wp.Name, ": ", err.Error())
		return false, err
	}
	return hId > 0, nil // return success and true if parameter was found
}

// DeleteWorksetParameter do delete workset parameter metadata and values from database.
func (mc *ModelCatalog) DeleteWorksetParameter(dn, wsn, name string) (bool, error) {

	// if model digest-or-name or workset name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if wsn == "" {
		omppLog.Log("Warning: invalid (empty) workset name")
		return false, nil
	}
	if name == "" {
		omppLog.Log("Warning: invalid (empty) workset parameter name")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return false, nil // return empty result: model not found or error
	}

	// lock catalog and update workset
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	// delete workset from database
	hId, err := db.DeleteWorksetParameter(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, wsn, name)
	if err != nil {
		omppLog.Log("Error at update workset: ", dn, ": ", wsn, ": ", err.Error())
		return false, err
	}
	return hId > 0, nil // return success and true if parameter was found
}
