// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"go.openmpp.org/ompp/db"
	"go.openmpp.org/ompp/helper"
	"go.openmpp.org/ompp/omppLog"
)

// write all readonly workset data into csv files: input parameters
func toWorksetListCsv(dbConn *sql.DB, modelDef *db.ModelMeta, outDir string, doubleFmt string, isIdCsv bool) error {

	// get all readonly worksets
	wl, err := db.GetWorksetFullList(dbConn, modelDef.Model.ModelId, true, "")
	if err != nil {
		return err
	}

	// read all workset parameters and dump it into csv files
	for k := range wl {
		err = toWorksetCsv(dbConn, modelDef, &wl[k], outDir, doubleFmt, isIdCsv)
		if err != nil {
			return err
		}
	}

	// write workset rows into csv
	row := make([]string, 6)

	idx := 0
	err = toCsvFile(
		outDir,
		"workset_lst.csv",
		[]string{"set_id", "base_run_id", "model_id", "set_name", "is_readonly", "update_dt"},
		func() (bool, []string, error) {
			if 0 <= idx && idx < len(wl) {
				row[0] = strconv.Itoa(wl[idx].Set.SetId)
				if wl[idx].Set.BaseRunId <= 0 { // non-positive run id is NULL
					row[1] = "NULL"
				} else {
					row[1] = strconv.Itoa(wl[idx].Set.BaseRunId)
				}
				row[2] = strconv.Itoa(wl[idx].Set.ModelId)
				row[3] = wl[idx].Set.Name
				row[4] = strconv.FormatBool(wl[idx].Set.IsReadonly)
				row[5] = wl[idx].Set.UpdateDateTime
				idx++
				return false, row, nil
			}
			return true, row, nil // end of workset rows
		})
	if err != nil {
		return errors.New("failed to write worksets into csv " + err.Error())
	}

	// write workset text rows into csv
	row = make([]string, 4)

	idx = 0
	j := 0
	err = toCsvFile(
		outDir,
		"workset_txt.csv",
		[]string{"set_id", "lang_code", "descr", "note"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(wl) { // end of workset rows
				return true, row, nil
			}

			// if end of current workset texts then find next workset with text rows
			if j < 0 || j >= len(wl[idx].Txt) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(wl) { // end of workset rows
						return true, row, nil
					}
					if len(wl[idx].Txt) > 0 {
						break
					}
				}
			}

			// make workset text []string row
			row[0] = strconv.Itoa(wl[idx].Txt[j].SetId)
			row[1] = wl[idx].Txt[j].LangCode
			row[2] = wl[idx].Txt[j].Descr

			if wl[idx].Txt[j].Note == "" { // empty "" string is NULL
				row[3] = "NULL"
			} else {
				row[3] = wl[idx].Txt[j].Note
			}
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write worksets text into csv " + err.Error())
	}

	// write workset parameter rows into csv
	row = make([]string, 2)

	idx = 0
	j = 0
	err = toCsvFile(
		outDir,
		"workset_parameter.csv",
		[]string{"set_id", "parameter_hid"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(wl) { // end of workset rows
				return true, row, nil
			}

			// if end of current workset parameters then find next workset with parameter rows
			if j < 0 || j >= len(wl[idx].Param) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(wl) { // end of workset rows
						return true, row, nil
					}
					if len(wl[idx].Param) > 0 {
						break
					}
				}
			}

			// make workset parameter []string row
			row[0] = strconv.Itoa(wl[idx].Set.SetId)
			row[1] = strconv.Itoa(wl[idx].Param[j].ParamHid)
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write workset parameters into csv " + err.Error())
	}

	// write workset parameter text (parameter value notes) rows into csv
	row = make([]string, 4)

	idx = 0
	pix := 0
	j = 0
	err = toCsvFile(
		outDir,
		"workset_parameter_txt.csv",
		[]string{"set_id", "parameter_hid", "lang_code", "note"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(wl) { // end of workset rows
				return true, row, nil
			}

			// if end of current workset parameter text then find next workset with parameter text rows
			if pix < 0 || pix >= len(wl[idx].Param) || j < 0 || j >= len(wl[idx].Param[pix].Txt) {

				j = 0
				for {
					if 0 <= pix && pix < len(wl[idx].Param) {
						pix++
					}
					if pix < 0 || pix >= len(wl[idx].Param) {
						idx++
						pix = 0
					}
					if idx < 0 || idx >= len(wl) { // end of workset rows
						return true, row, nil
					}
					if pix >= len(wl[idx].Param) { // end of parameter rows for that workset
						continue
					}
					if len(wl[idx].Param[pix].Txt) > 0 {
						break
					}
				}
			}

			// make workset parameter text []string row
			row[0] = strconv.Itoa(wl[idx].Param[pix].Txt[j].SetId)
			row[1] = strconv.Itoa(wl[idx].Param[pix].Txt[j].ParamHid)
			row[2] = wl[idx].Param[pix].Txt[j].LangCode

			if wl[idx].Param[pix].Txt[j].Note == "" { // empty "" string is NULL
				row[3] = "NULL"
			} else {
				row[3] = wl[idx].Param[pix].Txt[j].Note
			}
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write workset parameter text into csv " + err.Error())
	}

	return nil
}

// toWorksetCsv write workset paarameters into csv files, in separate subdirectory
func toWorksetCsv(
	dbConn *sql.DB, modelDef *db.ModelMeta, meta *db.WorksetMeta, outDir string, doubleFmt string, isIdCsv bool) error {

	// create workset subdir under output dir
	setId := meta.Set.SetId
	omppLog.Log("Workset ", setId, " ", meta.Set.Name)

	csvDir := filepath.Join(outDir, "set."+strconv.Itoa(setId)+"."+helper.ToAlphaNumeric(meta.Set.Name))

	err := os.MkdirAll(csvDir, 0750)
	if err != nil {
		return err
	}

	layout := &db.ReadLayout{FromId: setId, IsFromSet: true}

	// write parameter into csv file
	for j := range meta.Param {

		idx, ok := modelDef.ParamByHid(meta.Param[j].ParamHid)
		if !ok {
			return errors.New("missing workset parameter Hid: " + strconv.Itoa(meta.Param[j].ParamHid) + " workset: " + strconv.Itoa(layout.FromId) + " " + meta.Set.Name)
		}
		layout.Name = modelDef.Param[idx].Name

		cLst, err := db.ReadParameter(dbConn, modelDef, layout)
		if err != nil {
			return err
		}
		if cLst.Len() <= 0 { // parameter data must exist for all parameters
			return errors.New("missing workset parameter values " + layout.Name + " set id: " + strconv.Itoa(layout.FromId))
		}

		var pc db.CellValue
		err = toCsvCellFile(csvDir, modelDef, layout.Name, pc, cLst, doubleFmt, isIdCsv, "")
		if err != nil {
			return err
		}
	}

	return nil
}
