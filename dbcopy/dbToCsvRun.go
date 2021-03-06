// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/helper"
	"github.com/openmpp/go/ompp/omppLog"
)

// write all model run data into csv files: parameters, output expressions and accumulators
func toRunListCsv(
	dbConn *sql.DB,
	modelDef *db.ModelMeta,
	outDir string,
	doubleFmt string,
	isIdCsv bool,
	isWriteUtf8bom bool,
	doUseIdNames useIdNames,
	isAllInOne bool) error {

	// get all successfully completed model runs
	rl, err := db.GetRunFullTextList(dbConn, modelDef.Model.ModelId, true, "")
	if err != nil {
		return err
	}

	// read all run parameters, output accumulators and expressions and dump it into csv files
	for k := range rl {

		isUseIdNames := doUseIdNames == yesUseIdNames // usage of id's to make names: yes, no, default
		if doUseIdNames == defaultUseIdNames {
			for i := range rl {
				if isUseIdNames = i != k && rl[i].Run.Name == rl[k].Run.Name; isUseIdNames {
					break
				}
			}
		}

		err = toRunCsv(
			dbConn, modelDef, &rl[k], outDir, doubleFmt, isIdCsv, isWriteUtf8bom, isUseIdNames, k > 0, isAllInOne)
		if err != nil {
			return err
		}
	}

	// write model run rows into csv
	row := make([]string, 12)

	idx := 0
	err = toCsvFile(
		outDir,
		"run_lst.csv",
		isWriteUtf8bom,
		[]string{
			"run_id", "model_id", "run_name", "sub_count",
			"sub_started", "sub_completed", "create_dt", "status",
			"update_dt", "run_digest", "value_digest", "run_stamp"},
		func() (bool, []string, error) {
			if 0 <= idx && idx < len(rl) {
				row[0] = strconv.Itoa(rl[idx].Run.RunId)
				row[1] = strconv.Itoa(rl[idx].Run.ModelId)
				row[2] = rl[idx].Run.Name
				row[3] = strconv.Itoa(rl[idx].Run.SubCount)
				row[4] = strconv.Itoa(rl[idx].Run.SubStarted)
				row[5] = strconv.Itoa(rl[idx].Run.SubCompleted)
				row[6] = rl[idx].Run.CreateDateTime
				row[7] = rl[idx].Run.Status
				row[8] = rl[idx].Run.UpdateDateTime
				row[9] = rl[idx].Run.RunDigest
				row[9] = rl[idx].Run.ValueDigest
				row[10] = rl[idx].Run.RunStamp
				idx++
				return false, row, nil
			}
			return true, row, nil // end of model run rows
		})
	if err != nil {
		return errors.New("failed to write model run into csv " + err.Error())
	}

	// write model run text rows into csv
	row = make([]string, 4)

	idx = 0
	j := 0
	err = toCsvFile(
		outDir,
		"run_txt.csv",
		isWriteUtf8bom,
		[]string{"run_id", "lang_code", "descr", "note"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(rl) { // end of run rows
				return true, row, nil
			}

			// if end of current run texts then find next run with text rows
			if j < 0 || j >= len(rl[idx].Txt) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(rl) { // end of run rows
						return true, row, nil
					}
					if len(rl[idx].Txt) > 0 {
						break
					}
				}
			}

			// make model run text []string row
			row[0] = strconv.Itoa(rl[idx].Txt[j].RunId)
			row[1] = rl[idx].Txt[j].LangCode
			row[2] = rl[idx].Txt[j].Descr

			if rl[idx].Txt[j].Note == "" { // empty "" string is NULL
				row[3] = "NULL"
			} else {
				row[3] = rl[idx].Txt[j].Note
			}
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write model run text into csv " + err.Error())
	}

	// convert run option map to array of (id,key,value) rows
	var kvArr [][]string
	k := 0
	for j := range rl {
		for key, val := range rl[j].Opts {
			kvArr = append(kvArr, make([]string, 3))
			kvArr[k][0] = strconv.Itoa(rl[j].Run.RunId)
			kvArr[k][1] = key
			kvArr[k][2] = val
			k++
		}
	}

	// write model run option rows into csv
	row = make([]string, 3)

	idx = 0
	err = toCsvFile(
		outDir,
		"run_option.csv",
		isWriteUtf8bom,
		[]string{"run_id", "option_key", "option_value"},
		func() (bool, []string, error) {
			if 0 <= idx && idx < len(kvArr) {
				row = kvArr[idx]
				idx++
				return false, row, nil
			}
			return true, row, nil // end of run rows
		})
	if err != nil {
		return errors.New("failed to write model run text into csv " + err.Error())
	}

	// write run parameter rows into csv
	row = make([]string, 3)

	idx = 0
	j = 0
	err = toCsvFile(
		outDir,
		"run_parameter.csv",
		isWriteUtf8bom,
		[]string{"run_id", "parameter_hid", "sub_count"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(rl) { // end of model run rows
				return true, row, nil
			}

			// if end of current run parameters then find next run with parameter rows
			if j < 0 || j >= len(rl[idx].Param) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(rl) { // end of run rows
						return true, row, nil
					}
					if len(rl[idx].Param) > 0 {
						break
					}
				}
			}

			// make run parameter []string row
			row[0] = strconv.Itoa(rl[idx].Run.RunId)
			row[1] = strconv.Itoa(rl[idx].Param[j].ParamHid)
			row[2] = strconv.Itoa(rl[idx].Param[j].SubCount)
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write run parameters into csv " + err.Error())
	}

	// write parameter value notes rows into csv
	row = make([]string, 4)

	idx = 0
	pix := 0
	j = 0
	err = toCsvFile(
		outDir,
		"run_parameter_txt.csv",
		isWriteUtf8bom,
		[]string{"run_id", "parameter_hid", "lang_code", "note"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(rl) { // end of model run rows
				return true, row, nil
			}

			// if end of current run parameter text then find next run with parameter text rows
			if pix < 0 || pix >= len(rl[idx].Param) || j < 0 || j >= len(rl[idx].Param[pix].Txt) {

				j = 0
				for {
					if 0 <= pix && pix < len(rl[idx].Param) {
						pix++
					}
					if pix < 0 || pix >= len(rl[idx].Param) {
						idx++
						pix = 0
					}
					if idx < 0 || idx >= len(rl) { // end of model run rows
						return true, row, nil
					}
					if pix >= len(rl[idx].Param) { // end of run parameter text rows for that run
						continue
					}
					if len(rl[idx].Param[pix].Txt) > 0 {
						break
					}
				}
			}

			// make run parameter text []string row
			row[0] = strconv.Itoa(rl[idx].Param[pix].Txt[j].RunId)
			row[1] = strconv.Itoa(rl[idx].Param[pix].Txt[j].ParamHid)
			row[2] = rl[idx].Param[pix].Txt[j].LangCode

			if rl[idx].Param[pix].Txt[j].Note == "" { // empty "" string is NULL
				row[3] = "NULL"
			} else {
				row[3] = rl[idx].Param[pix].Txt[j].Note
			}
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write model run parameter text into csv " + err.Error())
	}

	// write run output tables rows into csv
	row = make([]string, 2)

	idx = 0
	j = 0
	err = toCsvFile(
		outDir,
		"run_table.csv",
		isWriteUtf8bom,
		[]string{"run_id", "table_hid"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(rl) { // end of model run rows
				return true, row, nil
			}

			// if end of current run output tables then find next run with table rows
			if j < 0 || j >= len(rl[idx].Table) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(rl) { // end of run rows
						return true, row, nil
					}
					if len(rl[idx].Table) > 0 {
						break
					}
				}
			}

			// make run output table []string row
			row[0] = strconv.Itoa(rl[idx].Run.RunId)
			row[1] = strconv.Itoa(rl[idx].Table[j].TableHid)
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write run output tables into csv " + err.Error())
	}

	// write run progress rows into csv
	row = make([]string, 7)

	idx = 0
	j = 0
	err = toCsvFile(
		outDir,
		"run_progress.csv",
		isWriteUtf8bom,
		[]string{"run_id", "sub_id", "create_dt", "status", "update_dt", "progress_count", "progress_value"},
		func() (bool, []string, error) {

			if idx < 0 || idx >= len(rl) { // end of model run rows
				return true, row, nil
			}

			// if end of current run progress then find next run with progress rows
			if j < 0 || j >= len(rl[idx].Progress) {
				j = 0
				for {
					idx++
					if idx < 0 || idx >= len(rl) { // end of run rows
						return true, row, nil
					}
					if len(rl[idx].Param) > 0 {
						break
					}
				}
			}

			// make run progress []string row
			row[0] = strconv.Itoa(rl[idx].Run.RunId)
			row[1] = strconv.Itoa(rl[idx].Progress[j].SubId)
			row[2] = rl[idx].Progress[j].CreateDateTime
			row[3] = rl[idx].Progress[j].Status
			row[4] = rl[idx].Progress[j].UpdateDateTime
			row[5] = strconv.Itoa(rl[idx].Progress[j].Count)
			if doubleFmt != "" {
				row[6] = fmt.Sprintf(doubleFmt, rl[idx].Progress[j].Value)
			} else {
				row[6] = fmt.Sprint(rl[idx].Progress[j].Value)
			}
			j++
			return false, row, nil
		})
	if err != nil {
		return errors.New("failed to write run progress into csv " + err.Error())
	}

	return nil
}

// toRunCsv write model run metadata, parameters and output tables into csv files, in separate subdirectory
func toRunCsv(
	dbConn *sql.DB,
	modelDef *db.ModelMeta,
	meta *db.RunMeta,
	outDir string,
	doubleFmt string,
	isIdCsv bool,
	isWriteUtf8bom bool,
	isUseIdNames bool,
	isNextRun bool,
	isAllInOne bool) error {

	// create run subdir under model dir
	runId := meta.Run.RunId
	omppLog.Log("Model run ", runId, " ", meta.Run.Name)

	// make output directory as one of:
	// all_model_runs, run.Name_Of_the_Run, run.NN.Name_Of_the_Run
	var csvDir string
	if isAllInOne {
		csvDir = filepath.Join(outDir, "all_model_runs")
	} else {
		if !isUseIdNames {
			csvDir = filepath.Join(outDir, "run."+helper.CleanPath(meta.Run.Name))
		} else {
			csvDir = filepath.Join(outDir, "run."+strconv.Itoa(runId)+"."+helper.CleanPath(meta.Run.Name))
		}
	}

	err := os.MkdirAll(csvDir, 0750)
	if err != nil {
		return err
	}

	// if this is "all-in-one" output then first column is run id or run name
	var firstCol, firstVal string
	if isAllInOne {
		if isIdCsv {
			firstCol = "run_id"
			firstVal = strconv.Itoa(runId)
		} else {
			firstCol = "run_name"
			firstVal = meta.Run.Name
		}
	}

	// write all parameters into csv file
	paramLt := &db.ReadParamLayout{ReadLayout: db.ReadLayout{FromId: runId}}

	for j := range modelDef.Param {

		paramLt.Name = modelDef.Param[j].Name

		cLst, _, err := db.ReadParameter(dbConn, modelDef, paramLt)
		if err != nil {
			return err
		}
		if cLst.Len() <= 0 { // parameter data must exist for all parameters
			return errors.New("missing run parameter values " + paramLt.Name + " run id: " + strconv.Itoa(paramLt.FromId))
		}

		var pc db.CellParam
		err = toCsvCellFile(
			csvDir,
			modelDef,
			paramLt.Name,
			isNextRun && isAllInOne,
			pc,
			cLst,
			doubleFmt,
			isIdCsv,
			"",
			isWriteUtf8bom,
			firstCol,
			firstVal)
		if err != nil {
			return err
		}
	}

	// write each run parameter value notes into parameterName.LANG.md file
	if !isAllInOne {
		for j := range meta.Param {

			paramName := ""
			for i := range meta.Param[j].Txt {

				if meta.Param[j].Txt[i].LangCode != "" && meta.Param[j].Txt[i].Note != "" {

					// find parameter by name if this is a first note for that parameter
					if paramName == "" {
						k, ok := modelDef.ParamByHid(meta.Param[j].ParamHid)
						if !ok {
							return errors.New("parameter not found by Hid: " + strconv.Itoa(meta.Param[j].ParamHid))
						}
						paramName = modelDef.Param[k].Name
					}

					// write notes into parameterName.LANG.md file
					err = toMdFile(
						csvDir,
						paramName+"."+meta.Param[j].Txt[i].LangCode,
						isWriteUtf8bom, meta.Param[j].Txt[i].Note)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	// write output tables into csv file, if the table included in run results
	tblLt := &db.ReadTableLayout{ReadLayout: db.ReadLayout{FromId: runId}}

	for j := range modelDef.Table {

		// check if table exist in model run results
		var isFound bool
		for k := range meta.Table {
			isFound = meta.Table[k].TableHid == modelDef.Table[j].TableHid
			if isFound {
				break
			}
		}
		if !isFound {
			continue // skip table: it is suppressed and not in run results
		}

		// write output table expression values into csv file
		tblLt.Name = modelDef.Table[j].Name
		tblLt.IsAccum = false
		tblLt.IsAllAccum = false

		cLst, _, err := db.ReadOutputTable(dbConn, modelDef, tblLt)
		if err != nil {
			return err
		}

		var ec db.CellExpr
		err = toCsvCellFile(
			csvDir,
			modelDef,
			tblLt.Name,
			isNextRun && isAllInOne,
			ec,
			cLst,
			doubleFmt,
			isIdCsv,
			"",
			isWriteUtf8bom,
			firstCol,
			firstVal)
		if err != nil {
			return err
		}

		// write output table accumulators into csv file
		tblLt.IsAccum = true
		tblLt.IsAllAccum = false

		cLst, _, err = db.ReadOutputTable(dbConn, modelDef, tblLt)
		if err != nil {
			return err
		}

		var ac db.CellAcc
		err = toCsvCellFile(
			csvDir,
			modelDef,
			tblLt.Name,
			isNextRun && isAllInOne,
			ac,
			cLst,
			doubleFmt,
			isIdCsv,
			"",
			isWriteUtf8bom,
			firstCol,
			firstVal)
		if err != nil {
			return err
		}

		// write all accumulators view into csv file
		tblLt.IsAccum = true
		tblLt.IsAllAccum = true

		cLst, _, err = db.ReadOutputTable(dbConn, modelDef, tblLt)
		if err != nil {
			return err
		}

		var al db.CellAllAcc
		err = toCsvCellFile(
			csvDir,
			modelDef,
			tblLt.Name,
			isNextRun && isAllInOne,
			al,
			cLst,
			doubleFmt,
			isIdCsv,
			"",
			isWriteUtf8bom,
			firstCol,
			firstVal)
		if err != nil {
			return err
		}
	}

	return nil
}
